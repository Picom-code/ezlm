package integrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DeepSeek Harness checks (PLAN 6.5). The model-and-tool check runs the
// headless profile in a throwaway DSH_HOME seeded with the exact files EZLM
// wrote, so no test session lands in the user's history. The preflight
// composes the real profiles with `--dump-config` (boot-free: nothing runs,
// no session starts) and proves EZLM's provider is in force and every
// provider the user had is still there.

// dshDefaultOverlay pins the check to EZLM's model without touching the
// user's default.
func dshDefaultOverlay(model string) []byte {
	b, _ := yamlEncode(&yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{
		yamlMap("id", yamlStr(dshModelRow), "config", yamlMap("provider", yamlStr(dshProvider), "model", yamlStr(model))),
	}})
	return b
}

// dshCopyFile copies src to dst when src exists.
func dshCopyFile(src, dst string, perm os.FileMode) error {
	b, err := os.ReadFile(src)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	return os.WriteFile(dst, b, perm)
}

// dshSeedHome makes a throwaway DSH_HOME holding the user's headless patch,
// the home-level patch (it applies to every profile) and the .env, so the
// check composes exactly what the user's headless runs would.
func dshSeedHome(home string) (string, error) {
	tmp, err := os.MkdirTemp("", "ezlm-dsh-check-")
	if err != nil {
		return "", err
	}
	for _, f := range []struct {
		src, dst string
		perm     os.FileMode
	}{
		{dshPatchPath(home, "headless"), dshPatchPath(tmp, "headless"), 0o644},
		{filepath.Join(home, "cordis.patch.yml"), filepath.Join(tmp, "cordis.patch.yml"), 0o644},
		{dshEnvPath(home), dshEnvPath(tmp), 0o600},
	} {
		if err := dshCopyFile(f.src, f.dst, f.perm); err != nil {
			os.RemoveAll(tmp)
			return "", err
		}
	}
	return tmp, nil
}

// dshEvents is what `dsh --profile headless --json` reported.
type dshEvents struct {
	final    string // the final message
	toolCall bool   // the agent called a tool
	errMsg   string // the turn's error, when it ended in one
}

// dshParseEvents reads headless's JSON lines; other lines (stderr) are
// skipped.
func dshParseEvents(out string) dshEvents {
	var ev dshEvents
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, "{") {
			continue
		}
		var e struct {
			Type   string `json:"type"`
			Text   string `json:"text"`
			Phase  string `json:"phase"`
			Reason struct {
				Kind  string `json:"kind"`
				Error struct {
					Message string `json:"message"`
					Code    string `json:"code"`
				} `json:"error"`
			} `json:"reason"`
		}
		if json.Unmarshal([]byte(l), &e) != nil {
			continue
		}
		switch e.Type {
		case "tool_call":
			ev.toolCall = true
		case "final":
			ev.final = e.Text
		case "status":
			if e.Phase == "turn_end" && e.Reason.Kind == "error" {
				ev.errMsg = strings.TrimSpace(e.Reason.Error.Code + ": " + e.Reason.Error.Message)
			}
		}
	}
	return ev
}

func (dshTool) Check(ctx context.Context, f Found, ep Endpoint, w *CheckWorkspace, r *Run) (bool, error) {
	if v := os.Getenv(dshKeyEnv); v != "" && v != dshKeyValue(ep.Key) {
		// dsh prefers the launch environment over every .env file.
		return false, errors.New("Your environment sets EZLM_API_KEY to an old key, and DeepSeek Harness uses it before EZLM's. Remove it from your shell profile, then press Connect again")
	}
	node, script, err := dshRuntime(f)
	if err != nil {
		return false, err
	}
	tmp, err := dshSeedHome(dshHome())
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmp)
	overlay := filepath.Join(tmp, "ezlm-default-model.patch.yml")
	if err := os.WriteFile(overlay, dshDefaultOverlay(ep.ModelID), 0o644); err != nil {
		return false, err
	}
	cmd := dshCommand(node, script, tmp, "--profile", "headless", "--patch", overlay, "--json", w.Prompt())
	cmd.Dir = w.Dir
	out, err := r.Exec(ctx, 10*time.Minute, cmd)
	ev := dshParseEvents(out)
	if w.Passed(ev.final) {
		return ev.toolCall, nil
	}
	if e, ok := Explain("dsh", out); ok {
		return ev.toolCall, errors.New(e.Say)
	}
	switch {
	case ctx.Err() != nil:
		return ev.toolCall, ctx.Err()
	case ev.errMsg != "":
		return ev.toolCall, fmt.Errorf("DeepSeek Harness stopped with an error (%s). Details are in %s", ev.errMsg, r.LogPath())
	case err != nil && ev.final == "":
		return ev.toolCall, fmt.Errorf("DeepSeek Harness stopped with an error (%s). Details are in %s", ErrLine(out), r.LogPath())
	case !ev.toolCall:
		return false, errors.New("The model answered DeepSeek Harness without using its file tool. Try a model that is better at calling tools, then connect again")
	}
	return true, errors.New("The model used DeepSeek Harness's file tool but didn't return the file's text. Try a model that is better at calling tools, then connect again")
}

func (dshTool) Preflight(ctx context.Context, f Found, ep Endpoint, r *Run) error {
	node, script, err := dshRuntime(f)
	if err != nil {
		return err
	}
	home := dshHome()
	for _, p := range dshManagedProfiles(home) {
		path := dshPatchPath(home, p)
		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("EZLM's settings for DeepSeek Harness are missing from %s. Press Connect to write them again", path)
		}
		d, err := dshParse(src)
		if err != nil {
			return fmt.Errorf("DeepSeek Harness's %s isn't valid YAML. Fix or remove it, then press Connect again", path)
		}
		have := dshProviderNames(dshProviders(d.root()))
		if p == "desktop" {
			// dsh's command line leaves this profile to the desktop app, so
			// the file itself is checked.
			if err := dshCheckRoute(dshProviders(d.root()), ep, have, path); err != nil {
				return err
			}
			continue
		}
		cmd := dshCommand(node, script, home, "--profile", p, "--dump-config")
		cmd.Dir = dshRunDir()
		out, err := r.Exec(ctx, 90*time.Second, cmd)
		if err != nil {
			if e, ok := Explain("dsh", out); ok {
				return errors.New(e.Say)
			}
			return fmt.Errorf("DeepSeek Harness couldn't read its %s settings (%s). Details are in %s", p, ErrLine(out), r.LogPath())
		}
		if err := dshCheckDump(out, ep, have, path, home); err != nil {
			return err
		}
	}
	return nil
}

// dshRunDir is the folder EZLM runs dsh's command line in: the workspace,
// or the home folder before the workspace exists. dsh also reads a .env
// from it.
func dshRunDir() string {
	if st, err := os.Stat(Workspace()); err == nil && st.IsDir() {
		return Workspace()
	}
	home, _ := os.UserHomeDir()
	return home
}

// dshCheckDump reads a `--dump-config` print (stderr warnings first, then
// the composed rows) and checks EZLM's route in it.
func dshCheckDump(out string, ep Endpoint, have []string, path, home string) error {
	var body []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "dsh: ") {
			// "dsh: [<file>] patch: entry "llm-pi-ai" not found" and the like.
			if strings.Contains(l, `"`+dshPiAiRow+`"`) || strings.Contains(l, `"`+dshModelRow+`"`) {
				return fmt.Errorf("DeepSeek Harness skipped part of EZLM's settings: %s", strings.TrimPrefix(l, "dsh: "))
			}
			continue
		}
		body = append(body, l)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(strings.Join(body, "\n")), &doc); err != nil || len(doc.Content) == 0 {
		return fmt.Errorf("EZLM couldn't read DeepSeek Harness's settings print for %s", path)
	}
	prov := dshProviders(doc.Content[0])
	if dshPairIndex(prov, dshProvider) < 0 {
		if hp := filepath.Join(home, "cordis.patch.yml"); dshFileHasRow(hp, dshPiAiRow) {
			return fmt.Errorf("Your %s replaces DeepSeek Harness's provider list, so EZLM's provider doesn't take effect. Remove its llm-pi-ai entry or add EZLM's provider there, then press Connect again", hp)
		}
	}
	return dshCheckRoute(prov, ep, have, path)
}

// dshCheckRoute checks that providers has EZLM's route for ep and every
// provider in have.
func dshCheckRoute(prov *yaml.Node, ep Endpoint, have []string, path string) error {
	i := dshPairIndex(prov, dshProvider)
	if i < 0 {
		return fmt.Errorf("DeepSeek Harness doesn't see EZLM's provider from %s. Press Connect to write it again", path)
	}
	e := prov.Content[i+1]
	var model *yaml.Node
	if ms := yamlChild(e, "models"); ms != nil && ms.Kind == yaml.SequenceNode && len(ms.Content) > 0 {
		model = ms.Content[0]
	}
	url, id, cw := yamlChild(e, "baseURL"), yamlChild(model, "id"), yamlChild(model, "contextWindow")
	if url == nil || url.Value != ep.BaseURL || id == nil || id.Value != ep.ModelID || cw == nil || cw.Value != fmt.Sprint(ep.Ctx) {
		return fmt.Errorf("DeepSeek Harness's EZLM provider in %s points at an old address, model or context. Press Connect to update it", path)
	}
	var lost []string
	for _, n := range have {
		if dshPairIndex(prov, n) < 0 {
			lost = append(lost, n)
		}
	}
	if len(lost) > 0 {
		return fmt.Errorf("DeepSeek Harness no longer sees your %s provider from %s. Check that file, then press Connect again", strings.Join(lost, ", "), path)
	}
	return nil
}

// dshFileHasRow reports whether a patch file has a row for id.
func dshFileHasRow(path, id string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	d, err := dshParse(b)
	return err == nil && dshRowIndex(d.root(), id, "") >= 0
}

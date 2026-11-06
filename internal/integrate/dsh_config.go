package integrate

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// What EZLM writes for DeepSeek Harness (PLAN 6.4): an `ezlm` provider in the
// llm-pi-ai row of each managed profile's cordis.patch.yml, the key in
// $DSH_HOME/.env, and the stored key ref when dsh already has one. A patch
// row replaces that row's whole config, so EZLM merges into the row the
// profile already has and never writes the home-level cordis.patch.yml,
// which outranks every profile.

const (
	dshProvider = "ezlm"         // the provider route EZLM owns
	dshKeyEnv   = "EZLM_API_KEY" // its credential reference
	dshPiAiRow  = "llm-pi-ai"
	dshPiAiName = "@deepseek-ai/dsh-llm-pi-ai"
	dshModelRow = "agent-default-model"
	// dshTimeoutMs gives a local model time for long prompts: llm-pi-ai's
	// own stream idle timeout is far shorter than a big first prompt takes on
	// a laptop.
	dshTimeoutMs = 1800000
)

// dshManagedProfiles are the profiles EZLM keeps its provider in: the check
// (headless) and the browser UI (web), plus the desktop app's profile when
// the DeepSeek Harness app has made one (DECISIONS 4).
func dshManagedProfiles(home string) []string {
	ps := []string{"headless", "web"}
	if st, err := os.Stat(filepath.Join(home, "profiles", "desktop")); err == nil && st.IsDir() {
		ps = append(ps, "desktop")
	}
	return ps
}

func dshProfileDir(home, profile string) string { return filepath.Join(home, "profiles", profile) }

func dshPatchPath(home, profile string) string {
	return filepath.Join(dshProfileDir(home, profile), "cordis.patch.yml")
}

// dshMaxTokens is the output cap EZLM states for the model: a quarter of the
// window, at most 32K. llm-pi-ai otherwise assumes 32,768, half of a 64K
// window, and makes it every request's max_tokens.
func dshMaxTokens(ctx int) int {
	n := ctx / 4
	if n > 32768 {
		n = 32768
	}
	if n < 1024 {
		n = 1024
	}
	return n
}

// dshPiAiLevels are llm-pi-ai's thinking levels, lowest first ("off" aside).
var dshPiAiLevels = []string{"minimal", "low", "medium", "high", "xhigh", "max"}

// dshEfforts is the reasoningEfforts map for the levels the model's template
// accepts, or nil when there are none (DECISIONS 6). Each level is sent under
// its own name. "off" has no value, which llm-pi-ai reads as "send no
// reasoning_effort": it is the level dsh starts on, so the default request
// carries no effort and the server's own default applies.
func dshEfforts(r Reasoning) *yaml.Node {
	m := yamlMap("off", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null"})
	for _, l := range dshPiAiLevels {
		if r.Accepts(l) {
			m.Content = append(m.Content, yamlStr(l), yamlStr(l))
		}
	}
	if len(m.Content) == 2 {
		return nil
	}
	return m
}

// dshProviderNode is EZLM's provider entry for ep. contextWindow and
// maxTokens are always stated: llm-pi-ai's fallbacks are 262,144 and 32,768.
func dshProviderNode(ep Endpoint) *yaml.Node {
	efforts := dshEfforts(ep.Reasoning)
	compat := yamlMap(
		"supportsStore", yamlBool(false),
		"supportsDeveloperRole", yamlBool(false),
		"maxTokensField", yamlStr("max_tokens"),
		"supportsUsageInStreaming", yamlBool(true),
		"supportsReasoningEffort", yamlBool(efforts != nil),
	)
	if efforts != nil {
		yamlSet(compat, "thinkingFormat", yamlStr("openai"))
	}
	input := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle, Content: []*yaml.Node{yamlStr("text")}}
	if ep.Vision {
		input.Content = append(input.Content, yamlStr("image"))
	}
	name := ep.ModelName
	if name == "" {
		name = ep.ModelID
	}
	model := yamlMap(
		"id", yamlStr(ep.ModelID),
		"name", yamlStr(name+" (EZLM)"),
		"contextWindow", yamlInt(ep.Ctx),
		"maxTokens", yamlInt(dshMaxTokens(ep.Ctx)),
		"input", input,
	)
	if efforts != nil {
		yamlSet(model, "reasoningEfforts", efforts)
	} else {
		yamlSet(model, "reasoningEfforts", yamlBool(false))
	}
	return yamlMap(
		"displayName", yamlStr("EZLM (local)"),
		"apiKeyEnv", yamlStr(dshKeyEnv),
		"api", yamlStr("openai-completions"),
		"baseURL", yamlStr(ep.BaseURL),
		"timeoutMs", yamlInt(dshTimeoutMs),
		"streamIdleTimeoutMs", yamlInt(dshTimeoutMs),
		"compat", compat,
		"models", &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{model}},
	)
}

// dshRowIndex is the index of the row a patch for id lands on, the way dsh's
// own settings editor picks it: the last mapping with that id that isn't an
// insert and names no other plugin. -1 if none.
func dshRowIndex(seq *yaml.Node, id, name string) int {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return -1
	}
	for i := len(seq.Content) - 1; i >= 0; i-- {
		row := seq.Content[i]
		if row.Kind != yaml.MappingNode {
			continue
		}
		if v := yamlChild(row, "id"); v == nil || v.Value != id || yamlChild(row, "insert") != nil {
			continue
		}
		if n := yamlChild(row, "name"); n != nil && name != "" && n.Value != name {
			continue
		}
		return i
	}
	return -1
}

// dshProviders is the providers mapping of the patch's llm-pi-ai row, or nil.
func dshProviders(root *yaml.Node) *yaml.Node {
	i := dshRowIndex(root, dshPiAiRow, dshPiAiName)
	if i < 0 {
		return nil
	}
	p := yamlChild(yamlChild(root.Content[i], "config"), "providers")
	if p == nil || p.Kind != yaml.MappingNode {
		return nil
	}
	return p
}

// dshProviderNames lists the provider routes in a providers mapping, except
// EZLM's own.
func dshProviderNames(p *yaml.Node) []string {
	var out []string
	if p == nil || p.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(p.Content); i += 2 {
		if k := p.Content[i].Value; k != dshProvider {
			out = append(out, k)
		}
	}
	return out
}

// dshEmptyList is a document holding an empty top-level list.
func dshEmptyList() *yaml.Node {
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.SequenceNode}}}
}

// dshUpsertProvider puts EZLM's provider entry into the patch's llm-pi-ai
// row, creating the row (or its config or providers) only when missing.
// Everything else in the file keeps its bytes.
func dshUpsertProvider(src []byte, entry *yaml.Node) ([]byte, error) {
	d, err := dshParse(src)
	if err != nil {
		return nil, err
	}
	if r := d.root(); r != nil && r.Kind != yaml.SequenceNode && !(r.Kind == yaml.ScalarNode && r.ShortTag() == "!!null") {
		return nil, errors.New("it isn't a list of patch rows")
	}
	// The same edit on a second parse is what the text edit must produce.
	want := dshEmptyList()
	if w, _ := dshParse(src); w.root() != nil && w.root().Kind == yaml.SequenceNode {
		want = w.doc
	}
	wseq := want.Content[0]
	seq := d.root()
	if seq != nil && seq.Kind != yaml.SequenceNode {
		seq = nil // a document that is only comments, or null
	}

	i := dshRowIndex(seq, dshPiAiRow, dshPiAiName)
	if i < 0 {
		row := yamlMap("id", yamlStr(dshPiAiRow), "config", yamlMap("providers", yamlMap(dshProvider, entry)))
		wseq.Content = append(wseq.Content, row)
		e, eerr := d.appendRow(row)
		return dshEdited(d, e, eerr, want)
	}
	row, wrow := seq.Content[i], wseq.Content[i]
	ci := dshPairIndex(row, "config")
	if ci < 0 {
		v := yamlMap("providers", yamlMap(dshProvider, entry))
		yamlSet(wrow, "config", v)
		e, eerr := d.appendPair(row, "config", v)
		return dshEdited(d, e, eerr, want)
	}
	cfg, wcfg := row.Content[ci+1], wrow.Content[ci+1]
	if !dshBlock(cfg) {
		// config: null, {} or a flow mapping: rewrite the config pair.
		v := yamlMap("providers", yamlMap(dshProvider, entry))
		if wcfg.Kind == yaml.MappingNode {
			v = wcfg
			v.Style &^= yaml.FlowStyle // the user's values keep their style
			if wp := yamlChild(v, "providers"); wp != nil && wp.Kind == yaml.MappingNode {
				wp.Style &^= yaml.FlowStyle
				dshSetProvider(wp, entry)
			} else {
				yamlSet(v, "providers", yamlMap(dshProvider, entry))
			}
		}
		wrow.Content[ci+1] = v
		e, eerr := d.replacePair(row, ci, v)
		return dshEdited(d, e, eerr, want)
	}
	pi := dshPairIndex(cfg, "providers")
	if pi < 0 {
		v := yamlMap(dshProvider, entry)
		yamlSet(wcfg, "providers", v)
		e, eerr := d.appendPair(cfg, "providers", v)
		return dshEdited(d, e, eerr, want)
	}
	prov, wprov := cfg.Content[pi+1], wcfg.Content[pi+1]
	if !dshBlock(prov) {
		v := yamlMap(dshProvider, entry)
		if wprov.Kind == yaml.MappingNode {
			v = wprov
			v.Style &^= yaml.FlowStyle
			dshSetProvider(v, entry)
		}
		wcfg.Content[pi+1] = v
		e, eerr := d.replacePair(cfg, pi, v)
		return dshEdited(d, e, eerr, want)
	}
	dshSetProvider(wprov, entry)
	var e dshEdit
	var eerr error
	if ei := dshPairIndex(prov, dshProvider); ei >= 0 {
		e, eerr = d.replacePair(prov, ei, entry)
	} else {
		e, eerr = d.appendPair(prov, dshProvider, entry)
	}
	return dshEdited(d, e, eerr, want)
}

func dshSetProvider(providers, entry *yaml.Node) { yamlSet(providers, dshProvider, entry) }

// dshSetDefault edits the agent-default-model row. With makeDefault it
// points the row at EZLM's model (creating it if missing). Without, it only
// follows a model switch when the row already uses EZLM, and drops a
// reasoning level the new model's template would reject.
func dshSetDefault(src []byte, ep Endpoint, makeDefault bool) ([]byte, error) {
	d, err := dshParse(src)
	if err != nil {
		return nil, err
	}
	w, _ := dshParse(src)
	seq := d.root()
	if seq == nil || seq.Kind != yaml.SequenceNode {
		if !makeDefault {
			return src, nil
		}
		seq = nil
	}
	want := dshEmptyList()
	if w.root() != nil && w.root().Kind == yaml.SequenceNode {
		want = w.doc
	}
	wseq := want.Content[0]
	cfg := yamlMap("provider", yamlStr(dshProvider), "model", yamlStr(ep.ModelID))
	i := dshRowIndex(seq, dshModelRow, "")
	if i < 0 {
		if !makeDefault {
			return src, nil
		}
		row := yamlMap("id", yamlStr(dshModelRow), "config", cfg)
		wseq.Content = append(wseq.Content, row)
		e, eerr := d.appendRow(row)
		return dshEdited(d, e, eerr, want)
	}
	row, wrow := seq.Content[i], wseq.Content[i]
	ci := dshPairIndex(row, "config")
	if !makeDefault {
		if ci < 0 {
			return src, nil
		}
		old := row.Content[ci+1]
		if p := yamlChild(old, "provider"); p == nil || p.Value != dshProvider {
			return src, nil // someone else's default
		}
		next := wrow.Content[ci+1]
		yamlSet(next, "model", yamlStr(ep.ModelID))
		if l := yamlChild(next, "reasoningEffort"); l != nil && !ep.Reasoning.Accepts(l.Value) {
			yamlDelete(next, "reasoningEffort")
		}
		if dshSame(old, next) {
			return src, nil
		}
		e, eerr := d.replacePair(row, ci, next)
		return dshEdited(d, e, eerr, want)
	}
	if ci < 0 {
		yamlSet(wrow, "config", cfg)
		e, eerr := d.appendPair(row, "config", cfg)
		return dshEdited(d, e, eerr, want)
	}
	wrow.Content[ci+1] = cfg
	e, eerr := d.replacePair(row, ci, cfg)
	return dshEdited(d, e, eerr, want)
}

// dshPatchTemplate is the header dsh writes into a new profile's patch file.
const dshPatchTemplate = "# Your patch layer for this dsh profile, applied after every bundle layer:\n" +
	"# a top-level YAML array of loader patch entries (id-targeted config\n" +
	"# overrides, disables, and insert lists; `!!js` expressions allowed).\n[]\n"

// WriteDshProfile merges EZLM's provider (and, with makeDefault, the default
// model) into one profile's cordis.patch.yml, under the profile's writer
// lock that dsh's own settings editor takes. It returns the provider routes
// the file had besides EZLM's, every one of which is checked to have kept
// its exact bytes.
func WriteDshProfile(path string, ep Endpoint, makeDefault bool) ([]string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var others []string
	err := dshWithLock(filepath.Join(dir, "package.json"), 10*time.Second, func() error {
		src, err := os.ReadFile(path)
		perm := os.FileMode(0o644)
		if os.IsNotExist(err) {
			src = []byte(dshPatchTemplate)
		} else if err != nil {
			return err
		} else if st, err := os.Stat(path); err == nil {
			perm = st.Mode().Perm()
		}
		before, err := dshParse(src)
		if err != nil {
			return fmt.Errorf("DeepSeek Harness's %s isn't valid YAML (%v). Fix or remove it, then press Connect again", path, err)
		}
		out, err := dshUpsertProvider(src, dshProviderNode(ep))
		if err == nil {
			out, err = dshSetDefault(out, ep, makeDefault)
		}
		if err != nil {
			return fmt.Errorf("EZLM couldn't add its provider to %s: %v. Fix or remove the file, then press Connect again", path, err)
		}
		others = dshProviderNames(dshProviders(before.root()))
		if err := dshKeptProviders(src, out, others); err != nil {
			return fmt.Errorf("EZLM stopped before changing %s: %v", path, err)
		}
		if bytes.Equal(out, src) {
			if _, err := os.Stat(path); err == nil {
				return nil
			}
		}
		return WriteConfig(path, out, perm)
	})
	return others, err
}

// dshKeptProviders checks that every provider in names means the same in
// after as in before. The text edits leave their bytes alone; when a file's
// layout made EZLM encode the tree instead, the meaning still has to match.
func dshKeptProviders(before, after []byte, names []string) error {
	b, err := dshParse(before)
	if err != nil {
		return err
	}
	a, err := dshParse(after)
	if err != nil {
		return err
	}
	bp, ap := dshProviders(b.root()), dshProviders(a.root())
	for _, n := range names {
		bi, ai := dshPairIndex(bp, n), dshPairIndex(ap, n)
		if ai < 0 || !dshSame(bp.Content[bi+1], ap.Content[ai+1]) {
			return fmt.Errorf("the %s provider would have changed", n)
		}
	}
	return nil
}

// pairText is the source text of m's pair at index i.
func (d *dshDoc) pairText(m *yaml.Node, i int) []byte {
	k := m.Content[i]
	return d.src[d.offset(k.Line, k.Column):d.offset(d.pairEnd(m, i), 1)]
}

// dshEnvPath is the home-level .env, the lowest layer dsh reads keys from.
func dshEnvPath(home string) string { return filepath.Join(home, ".env") }

var (
	dshEnvLine  = regexp.MustCompile(`^\s*(?:export\s+)?` + dshKeyEnv + `\s*=`)
	dshEnvPlain = regexp.MustCompile(`^[A-Za-z0-9._~+/=:-]*$`)
)

// dshEnvValue renders a .env value: bare when it's plain, else double-quoted.
func dshEnvValue(v string) string {
	if dshEnvPlain.MatchString(v) {
		return v
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(v) + `"`
}

// dshMergeEnv sets EZLM_API_KEY in a .env file's text, keeping every other
// line (and its comments) as it was.
func dshMergeEnv(src []byte, key string) []byte {
	line := dshKeyEnv + "=" + dshEnvValue(key)
	nl := "\n"
	if bytes.Contains(src, []byte("\r\n")) {
		nl = "\r\n"
	}
	text := string(src)
	lines := strings.SplitAfter(text, "\n")
	found := false
	var out strings.Builder
	for _, l := range lines {
		if l == "" {
			continue
		}
		body := strings.TrimRight(l, "\r\n")
		if dshEnvLine.MatchString(body) {
			if found {
				continue // a later duplicate would win; drop it
			}
			found = true
			out.WriteString(line + l[len(body):])
			continue
		}
		out.WriteString(l)
	}
	if !found {
		if out.Len() > 0 && !strings.HasSuffix(out.String(), "\n") {
			out.WriteString(nl)
		}
		out.WriteString(line + nl)
	}
	return []byte(out.String())
}

// dshEnvKey reads EZLM_API_KEY from a .env file's text ("" if absent).
func dshEnvKey(src []byte) string {
	for _, l := range strings.Split(string(src), "\n") {
		l = strings.TrimRight(l, "\r")
		if !dshEnvLine.MatchString(l) {
			continue
		}
		v := strings.TrimSpace(l[strings.Index(l, "=")+1:])
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			q := v[0]
			v = v[1 : len(v)-1]
			if q == '"' {
				v = strings.NewReplacer(`\n`, "\n", `\"`, `"`, `\\`, `\`).Replace(v)
			}
		} else if i := strings.Index(v, " #"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		return v
	}
	return ""
}

// WriteDshEnv puts the key into $DSH_HOME/.env (mode 0600).
func WriteDshEnv(home, key string) error {
	path := dshEnvPath(home)
	src, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	out := dshMergeEnv(src, key)
	if err == nil && bytes.Equal(out, src) {
		return os.Chmod(path, 0o600)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	return WriteConfig(path, out, 0o600)
}

// dshCredentialsPath is dsh's managed credential store. A ref stored there
// (an in-app key save) wins over the launch environment and every .env, so
// EZLM keeps its own ref current when one exists and never adds one.
func dshCredentialsPath(home string) string { return filepath.Join(home, ".credentials.yaml") }

// dshStoredRef reads refs.EZLM_API_KEY from the credential store: the value,
// and whether the ref exists.
func dshStoredRef(home string) (string, bool) {
	b, err := os.ReadFile(dshCredentialsPath(home))
	if err != nil {
		return "", false
	}
	d, err := dshParse(b)
	if err != nil || d.root() == nil {
		return "", false
	}
	v := yamlChild(yamlChild(d.root(), "refs"), dshKeyEnv)
	if v == nil || v.Kind != yaml.ScalarNode {
		return "", false
	}
	return v.Value, true
}

// UpdateDshStoredRef rewrites an existing refs.EZLM_API_KEY to key, the way
// dsh's credential provider does: under the store's writer lock (a wx-created
// "<file>.lock" holding the writer's PID), re-reading the file first, and
// replacing it atomically at mode 0600. Every other line keeps its bytes. No
// backup is kept: copies of a credential store would outlive the keys they
// hold.
func UpdateDshStoredRef(home, key string) error {
	path := dshCredentialsPath(home)
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	return dshWithLock(path, 30*time.Second, func() error {
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		d, err := dshParse(src)
		if err != nil || d.root() == nil || d.root().Kind != yaml.MappingNode {
			return nil // dsh refuses such a store at boot and says why
		}
		refs := yamlChild(d.root(), "refs")
		i := dshPairIndex(refs, dshKeyEnv)
		if i < 0 || refs.Content[i+1].Value == key {
			return nil
		}
		w, _ := dshParse(src)
		wv := yamlChild(yamlChild(w.root(), "refs"), dshKeyEnv)
		wv.Value, wv.Tag, wv.Style = key, "!!str", 0
		e, eerr := d.replacePair(refs, i, yamlStr(key))
		if !dshBlock(refs) {
			eerr = errors.New("flow refs")
		}
		out, err := dshEdited(d, e, eerr, w.doc)
		if err != nil {
			return err
		}
		return WriteFileAtomic(path, out, 0o600)
	})
}

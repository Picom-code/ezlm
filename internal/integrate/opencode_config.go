package integrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tailscale/hujson"
)

// OpenCode's config, as EZLM writes it (PLAN 4.4). The schema is the 1.x
// one, which 1.19.5 reads natively and 2.x converts in memory. EZLM owns
// provider.ezlm, and model/small_model while they point at it; everything
// else in the file, comments and key order included, stays as it was.

// openCodeConfigDir is the global config folder OpenCode reads:
// $OPENCODE_CONFIG_DIR, else $XDG_CONFIG_HOME/opencode, else
// ~/.config/opencode, on every OS (OpenCode never uses %APPDATA%). 2.x reads
// only that folder; 1.x reads OPENCODE_CONFIG_DIR on top of the others, with
// the highest priority.
func openCodeConfigDir() string {
	if d := os.Getenv("OPENCODE_CONFIG_DIR"); d != "" {
		return d
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "opencode")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "opencode")
}

// openCodeConfigFile is the file EZLM edits: opencode.jsonc when it exists
// (both versions read it after opencode.json, so it wins), else
// opencode.json.
func openCodeConfigFile(dir string) string {
	if st, err := os.Stat(filepath.Join(dir, "opencode.jsonc")); err == nil && !st.IsDir() {
		return filepath.Join(dir, "opencode.jsonc")
	}
	return filepath.Join(dir, "opencode.json")
}

// openCodeKeyFile holds EZLM's key alone; the config references it with
// {file:}, so the key never sits in a file people share or paste.
func openCodeKeyFile(dir string) string { return filepath.Join(dir, "ezlm.key") }

// openCodeOutput is limit.output: a quarter of the context, at least 4096
// and at most 32768 tokens, and never more than half the context.
func openCodeOutput(ctx int) int {
	out := min(max(ctx/4, 4096), 32768)
	return max(min(out, ctx/2), 1)
}

// openCodeTimeoutMS is the request, header and chunk timeout: a long prefill
// on a CPU or hybrid setup can pass OpenCode's 300 s defaults before the
// first byte arrives.
const openCodeTimeoutMS = 1800000

// openCodeEntry is EZLM's provider entry for ep, in the key order EZLM
// writes. Reasoning variants are listed only for the levels the model's
// template accepts, so a selected variant can never send one it rejects;
// with no levels there are none, and no request carries reasoning_effort.
// reasoning, release_date and attachment are left out: 2.x drops them with
// warnings.
func openCodeEntry(ep Endpoint, keyFile string) openCodeObj {
	input := []string{"text"}
	if ep.Vision {
		input = append(input, "image")
	}
	model := openCodeObj{
		{"name", ep.ModelName + " (EZLM)"},
		{"tool_call", ep.ToolCalls},
		{"modalities", openCodeObj{{"input", input}, {"output", []string{"text"}}}},
		{"limit", openCodeObj{{"context", ep.Ctx}, {"output", openCodeOutput(ep.Ctx)}}},
	}
	if len(ep.Reasoning.Levels) > 0 {
		var vs openCodeObj
		for _, l := range ep.Reasoning.Levels {
			vs = append(vs, openCodeKV{l, openCodeObj{{"reasoningEffort", l}}})
		}
		model = append(model, openCodeKV{"variants", vs})
	}
	return openCodeObj{
		{"npm", "@ai-sdk/openai-compatible"},
		{"name", "EZLM (local)"},
		{"options", openCodeObj{
			{"baseURL", ep.BaseURL},
			{"apiKey", "{file:" + filepath.ToSlash(keyFile) + "}"},
			{"timeout", openCodeTimeoutMS},
			{"headerTimeout", openCodeTimeoutMS},
			{"chunkTimeout", openCodeTimeoutMS},
		}},
		{"models", openCodeObj{{ep.ModelID, model}}},
	}
}

// WriteOpenCodeConfig merges EZLM's provider into dir's opencode.jsonc or
// opencode.json, keeping every other setting and comment. The key goes to
// ezlm.key first, so a key rotation reaches OpenCode even when the config
// can't be edited.
func WriteOpenCodeConfig(dir string, ep Endpoint, makeDefault bool) error {
	keyFile := openCodeKeyFile(dir)
	if err := WriteFileAtomic(keyFile, []byte(ep.Key), 0o600); err != nil {
		return err
	}
	path := openCodeConfigFile(dir)
	orig, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	doc, err := openCodeParse(orig)
	if err != nil {
		return fmt.Errorf("%s isn't valid JSON (%v), so EZLM can't add itself to it. Fix or remove the file, then press Connect again.", path, err)
	}
	cur := openCodeStrings(doc)
	model := openCodeProvider + "/" + ep.ModelID
	doc.set([]string{"$schema"}, "https://opencode.ai/config.json", false)
	if makeDefault || cur["model"] == "" || strings.HasPrefix(cur["model"], openCodeProvider+"/") {
		doc.set([]string{"model"}, model, true)
	}
	if strings.HasPrefix(cur["small_model"], openCodeProvider+"/") {
		doc.set([]string{"small_model"}, model, true)
	}
	doc.set([]string{"provider", openCodeProvider}, openCodeEntry(ep, keyFile), true)
	out := doc.bytes()
	if bytes.Equal(out, orig) {
		return nil
	}
	return WriteConfig(path, out, 0o644)
}

// writeOpenCodeWorkspace points the workspace's project config at EZLM's
// model: OpenCode 2.x's TUI has no --model, so this file picks the model
// for sessions started there, without touching the user's global default.
func writeOpenCodeWorkspace(ep Endpoint) error {
	path := filepath.Join(Workspace(), "opencode.json")
	orig, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	doc, err := openCodeParse(orig)
	if err != nil {
		doc, _ = openCodeParse(nil) // EZLM's own file: start it again
	}
	doc.set([]string{"$schema"}, "https://opencode.ai/config.json", false)
	doc.set([]string{"model"}, openCodeProvider+"/"+ep.ModelID, true)
	out := doc.bytes()
	if bytes.Equal(out, orig) {
		return nil
	}
	return WriteFileAtomic(path, out, 0o644)
}

// openCodeConfig is what Detect and the preflight read back.
type openCodeConfig struct {
	Model      string `json:"model"`
	SmallModel string `json:"small_model"`
	Provider   map[string]struct {
		Options struct {
			BaseURL string `json:"baseURL"`
			APIKey  string `json:"apiKey"`
		} `json:"options"`
		Models map[string]struct {
			ToolCall *bool `json:"tool_call"`
			Limit    struct {
				Context int `json:"context"`
				Output  int `json:"output"`
			} `json:"limit"`
			Variants map[string]json.RawMessage `json:"variants"`
		} `json:"models"`
	} `json:"provider"`
}

// readOpenCodeConfig parses a config file (JSONC allowed); a missing file
// is an empty config.
func readOpenCodeConfig(path string) (openCodeConfig, error) {
	var c openCodeConfig
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	std, err := hujson.Standardize(b)
	if err != nil {
		return c, fmt.Errorf("%s isn't valid JSON, so OpenCode can't read it. Fix or remove it, then press Connect.", path)
	}
	if len(bytes.TrimSpace(std)) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(std, &c); err != nil {
		return c, fmt.Errorf("%s isn't valid JSON, so OpenCode can't read it. Fix or remove it, then press Connect.", path)
	}
	return c, nil
}

// linked: the config has EZLM's provider, whatever it points at.
func (c openCodeConfig) linked() bool {
	_, ok := c.Provider[openCodeProvider]
	return ok
}

// stale says why EZLM's entry doesn't match ep ("" when it does): another
// address, key, model, context, tool flag or level list, a default that
// points at a model EZLM no longer lists, or a workspace file that opens
// another model.
func (c openCodeConfig) stale(ep Endpoint, dir string) string {
	p := c.Provider[openCodeProvider]
	m, ok := p.Models[ep.ModelID]
	switch {
	case p.Options.BaseURL != ep.BaseURL:
		return "OpenCode points at an old EZLM address."
	case p.Options.APIKey != "{file:"+filepath.ToSlash(openCodeKeyFile(dir))+"}" || openCodeKeyMatches(dir, ep.Key) != nil:
		return "OpenCode has an old EZLM key."
	case !ok || len(p.Models) != 1:
		return "OpenCode lists another EZLM model."
	case m.Limit.Context != ep.Ctx || m.Limit.Output != openCodeOutput(ep.Ctx) || m.ToolCall == nil || *m.ToolCall != ep.ToolCalls:
		return "OpenCode has old limits for this model."
	case !openCodeSameLevels(m.Variants, ep.Reasoning.Levels):
		return "OpenCode offers thinking levels this model doesn't take."
	}
	for _, v := range []string{c.Model, c.SmallModel} {
		if strings.HasPrefix(v, openCodeProvider+"/") && v != openCodeProvider+"/"+ep.ModelID {
			return "OpenCode's default model is an old EZLM model."
		}
	}
	if w, err := readOpenCodeConfig(filepath.Join(Workspace(), "opencode.json")); err == nil && w.Model != "" && w.Model != openCodeProvider+"/"+ep.ModelID {
		return "OpenCode's EZLM Workspace opens an old EZLM model."
	}
	return ""
}

func openCodeSameLevels(vs map[string]json.RawMessage, levels []string) bool {
	if len(vs) != len(levels) {
		return false
	}
	for _, l := range levels {
		if _, ok := vs[l]; !ok {
			return false
		}
	}
	return true
}

// openCodeKeyMatches checks ezlm.key: OpenCode 2.x silently sends no key
// when the file is missing, so a missing or old file is an error.
func openCodeKeyMatches(dir, key string) error {
	b, err := os.ReadFile(openCodeKeyFile(dir))
	if err != nil || strings.TrimSpace(string(b)) != key {
		return fmt.Errorf("OpenCode has an old EZLM key. Press Connect to update it.")
	}
	return nil
}

// openCodeObj is a JSON object that keeps its key order.
type openCodeObj []openCodeKV

type openCodeKV struct {
	K string
	V any
}

func (o openCodeObj) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, kv := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := openCodeMarshal(kv.K)
		v, err := openCodeMarshal(kv.V)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// openCodeMarshal is json.Marshal without HTML escaping.
func openCodeMarshal(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// openCodeDoc is a parsed JSONC file being edited in place: values EZLM
// sets are replaced or appended where they belong, indented like the rest
// of the file, and every other byte (comments, trailing commas, key order,
// spacing) is kept.
type openCodeDoc struct {
	v      hujson.Value
	indent string
}

// openCodeParse parses b (empty means a new file).
func openCodeParse(b []byte) (*openCodeDoc, error) {
	if len(bytes.TrimSpace(b)) == 0 {
		b = []byte("{\n}\n")
	}
	v, err := hujson.Parse(b)
	if err != nil {
		return nil, err
	}
	if _, ok := v.Value.(*hujson.Object); !ok {
		return nil, fmt.Errorf("the top level isn't an object")
	}
	return &openCodeDoc{v: v, indent: openCodeIndent(b)}, nil
}

// openCodeIndent is the file's indent unit: the leading whitespace of its
// first indented line, else two spaces.
func openCodeIndent(b []byte) string {
	for _, line := range strings.Split(string(b), "\n") {
		t := strings.TrimLeft(line, " \t")
		if t != "" && len(t) < len(line) {
			return line[:len(line)-len(t)]
		}
	}
	return "  "
}

// openCodeStrings reads the top-level string settings EZLM's rules look at.
func openCodeStrings(d *openCodeDoc) map[string]string {
	out := map[string]string{}
	obj := d.v.Value.(*hujson.Object)
	for _, m := range obj.Members {
		var s string
		if json.Unmarshal(m.Name.Value.(hujson.Literal), &s) != nil {
			continue
		}
		if lit, ok := m.Value.Value.(hujson.Literal); ok {
			var v string
			if json.Unmarshal(lit, &v) == nil {
				out[s] = v
			}
		}
	}
	return out
}

// set puts val at path (object keys), creating objects on the way. An
// existing value is replaced only when replace is set; its surrounding
// comments stay.
func (d *openCodeDoc) set(path []string, val any, replace bool) {
	obj := d.v.Value.(*hujson.Object)
	for i, key := range path {
		depth := i + 1
		idx := slices.IndexFunc(obj.Members, func(m hujson.ObjectMember) bool {
			var s string
			return json.Unmarshal(m.Name.Value.(hujson.Literal), &s) == nil && s == key
		})
		if idx >= 0 && i < len(path)-1 {
			if next, ok := obj.Members[idx].Value.Value.(*hujson.Object); ok {
				obj = next
				continue
			}
		}
		// The rest of the path becomes one value.
		v := val
		for j := len(path) - 1; j > i; j-- {
			v = openCodeObj{{path[j], v}}
		}
		if idx >= 0 {
			if !replace {
				return
			}
			old := obj.Members[idx].Value
			nv := d.value(v, depth, openCodeCompact(obj))
			nv.BeforeExtra, nv.AfterExtra = old.BeforeExtra, old.AfterExtra
			obj.Members[idx].Value = nv
			return
		}
		d.appendMember(obj, key, v, depth)
		return
	}
}

// appendMember adds key as obj's last member. A comment after the old last
// member stays on that member's line, and the closing brace keeps its place.
func (d *openCodeDoc) appendMember(obj *hujson.Object, key string, v any, depth int) {
	name, _ := openCodeMarshal(key)
	compact := openCodeCompact(obj)
	nv := d.value(v, depth, compact)
	nv.BeforeExtra = hujson.Extra(" ")
	before := "\n" + strings.Repeat(d.indent, depth)
	after := string(obj.AfterExtra)
	switch {
	case compact:
		before = ""
	case len(obj.Members) == 0 && strings.TrimSpace(after) == "":
		after = "\n" + strings.Repeat(d.indent, depth-1)
	default:
		if i := strings.LastIndexByte(after, '\n'); i >= 0 {
			before, after = after[:i]+before, after[i:]
		} else {
			before, after = after+before, "\n"+strings.Repeat(d.indent, depth-1)
		}
	}
	obj.AfterExtra = hujson.Extra(after)
	obj.Members = append(obj.Members, hujson.ObjectMember{
		Name:  hujson.Value{BeforeExtra: hujson.Extra(before), Value: hujson.Literal(name)},
		Value: nv,
	})
}

// openCodeCompact: an object written on one line, such as
// "limit": { "context": 65536 }, gets new members on that line too.
func openCodeCompact(obj *hujson.Object) bool {
	if len(obj.Members) == 0 || bytes.IndexByte(obj.AfterExtra, '\n') >= 0 {
		return false
	}
	for _, m := range obj.Members {
		if bytes.IndexByte(m.Name.BeforeExtra, '\n') >= 0 {
			return false
		}
	}
	return true
}

// value renders v as a HuJSON value for a member at depth: indented like
// the file, or on one line in a compact object.
func (d *openCodeDoc) value(v any, depth int, oneLine bool) hujson.Value {
	compact, _ := openCodeMarshal(v)
	var pretty bytes.Buffer
	if oneLine {
		pretty.Write(compact)
	} else {
		json.Indent(&pretty, compact, strings.Repeat(d.indent, depth), d.indent)
	}
	hv, err := hujson.Parse(pretty.Bytes())
	if err != nil {
		hv, _ = hujson.Parse(compact)
	}
	hv.BeforeExtra, hv.AfterExtra = nil, nil
	return hv
}

// bytes is the edited file, ending in one newline.
func (d *openCodeDoc) bytes() []byte {
	d.v.UpdateOffsets()
	return append(bytes.TrimRight(d.v.Pack(), " \t\r\n"), '\n')
}

package integrate

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Hermes's config.yaml: where it is and what EZLM writes into it.
//
// From _config_version 12 on, Hermes keeps providers in a `providers:` map
// and reads the older `custom_providers:` list first, so EZLM writes
// providers.ezlm (it resolves as custom:ezlm) and removes its own legacy
// list entry. Older files get the legacy entry. Matching is on parsed names,
// never text, so a user's "ezlm-old" is never touched.

// hermesEZLMMark tags the reasoning override EZLM writes, so a later write
// can tell it from one the user set.
const hermesEZLMMark = "# EZLM:"

// hermesConfigPath is the config.yaml the Hermes at bin reads: what
// `hermes config path` printed (cached with the program's probe) while the
// environment that decides it is unchanged, else Hermes's own rules.
func hermesConfigPath(bin string) string {
	if bin != "" {
		if p, ok := CachedProbe(bin); ok && p.Facts["config"] != "" && p.Facts["env"] == hermesEnvKey() {
			return p.Facts["config"]
		}
	}
	return hermesDefaultConfig()
}

// hermesDefaultConfig follows Hermes's rules: $HERMES_HOME, else the
// platform default (%LOCALAPPDATA%\hermes on Windows, created on first
// write, never ~/.hermes there), then a sticky `active_profile` in that
// root unless HERMES_HOME already names a profile.
func hermesDefaultConfig() string {
	home := hermesHomeEnv()
	if home != "" && filepath.Base(filepath.Dir(home)) == "profiles" {
		return filepath.Join(home, "config.yaml")
	}
	root := home
	if root == "" {
		root = hermesNativeHome()
	}
	if name := hermesActiveProfile(root); name != "" {
		if st, err := os.Stat(filepath.Join(root, "profiles", name)); err == nil && st.IsDir() {
			return filepath.Join(root, "profiles", name, "config.yaml")
		}
	}
	return filepath.Join(root, "config.yaml")
}

// hermesNativeHome is Hermes's platform default home, with the literal
// HERMES_DATA_DIR_SUFFIX.
func hermesNativeHome() string {
	suffix := os.Getenv("HERMES_DATA_DIR_SUFFIX")
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		la := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		if la == "" {
			la = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(la, "hermes"+suffix)
	}
	return filepath.Join(home, ".hermes"+suffix)
}

var hermesEnvRef = regexp.MustCompile(`%([^%]+)%`)

// hermesHomeRaw is HERMES_HOME as EZLM's own environment has it, else as
// the user's login shell (Mac, Linux) or saved Windows environment sets it,
// so a HERMES_HOME exported only in ~/.zshrc still counts.
func hermesHomeRaw() string {
	if h := os.Getenv("HERMES_HOME"); h != "" {
		return h
	}
	return LoginEnv("HERMES_HOME")
}

// hermesHomeEnv is $HERMES_HOME expanded the way Hermes expands it, or "".
func hermesHomeEnv() string {
	h := strings.TrimSpace(hermesHomeRaw())
	if h == "" {
		return ""
	}
	if runtime.GOOS == "windows" {
		h = hermesEnvRef.ReplaceAllStringFunc(h, func(m string) string { return os.Getenv(m[1 : len(m)-1]) })
	}
	h = os.ExpandEnv(h)
	if h == "~" || strings.HasPrefix(h, "~/") || strings.HasPrefix(h, `~\`) {
		home, _ := os.UserHomeDir()
		h = home + h[1:]
	}
	return filepath.Clean(h)
}

// hermesActiveProfile is the sticky profile `hermes profile use` saved in
// root ("" for none or default).
func hermesActiveProfile(root string) string {
	b, err := os.ReadFile(filepath.Join(root, "active_profile"))
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(strings.TrimPrefix(string(b), "\ufeff"))
	if name == "default" || strings.ContainsAny(name, `/\`) {
		return ""
	}
	return name
}

// hermesEnvKey is everything that decides which config Hermes reads, so a
// cached `hermes config path` answer is only used while it still holds.
func hermesEnvKey() string {
	home, _ := os.UserHomeDir()
	root := hermesHomeEnv()
	if root == "" {
		root = hermesNativeHome()
	}
	return strings.Join([]string{home, hermesHomeRaw(), os.Getenv("HERMES_DATA_DIR_SUFFIX"), os.Getenv("LOCALAPPDATA"), hermesActiveProfile(root)}, "|")
}

// WriteHermesConfig adds or updates EZLM's provider in a Hermes config.yaml,
// editing the YAML node tree so the user's comments and ordering survive.
// The `model:` block (Hermes's default) becomes EZLM's when makeDefault is
// set, when there is none, or when it already points at EZLM; it is never
// taken from another provider otherwise. A context below Hermes's 64K floor
// returns ErrNeeds64K and leaves the file as it is.
func WriteHermesConfig(path string, ep Endpoint, makeDefault bool) error {
	if ep.Ctx < hermesMinCtx {
		return ErrNeeds64K
	}
	doc, err := yamlLoad(path)
	if err != nil {
		return err
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("%s is not a YAML mapping", path)
	}
	if hermesModern(root) {
		hermesWriteProvider(root, ep)
	} else {
		hermesWriteLegacy(root, ep)
	}
	hermesWriteModel(root, ep, makeDefault)
	hermesWriteReasoning(root, ep)
	b, err := yamlEncode(doc)
	if err != nil {
		return err
	}
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, b) {
		return nil // already current: no backup, no write
	}
	return WriteConfig(path, b, 0o600)
}

// hermesModern: a file at _config_version 12 or later, or with no version
// (Hermes treats an unversioned file as current).
func hermesModern(root *yaml.Node) bool {
	v := yamlChild(root, "_config_version")
	if v == nil || v.Kind != yaml.ScalarNode {
		return true
	}
	n, err := strconv.Atoi(strings.TrimSpace(v.Value))
	return err != nil || n >= 12
}

// hermesIsEZLM reports whether a provider key or name means EZLM's
// provider, the way Hermes matches --provider custom:ezlm.
func hermesIsEZLM(name string) bool {
	n := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), " ", "-")
	return n == "ezlm" || n == "custom:ezlm"
}

func hermesScalar(m *yaml.Node, key string) string {
	if v := yamlChild(m, key); v != nil && v.Kind == yaml.ScalarNode {
		return v.Value
	}
	return ""
}

// hermesWriteProvider writes providers.ezlm, keeping any EZLM entry already
// there (under "ezlm" or "custom:ezlm") and the user's extra fields in it,
// and drops EZLM's legacy custom_providers entry, which would shadow it.
func hermesWriteProvider(root *yaml.Node, ep Endpoint) {
	provs := yamlChild(root, "providers")
	if provs == nil || provs.Kind != yaml.MappingNode {
		provs = &yaml.Node{Kind: yaml.MappingNode}
		yamlSet(root, "providers", provs)
	}
	var entry *yaml.Node
	for i := 0; i+1 < len(provs.Content); {
		k, v := provs.Content[i], provs.Content[i+1]
		if hermesIsEZLM(k.Value) || (v.Kind == yaml.MappingNode && hermesIsEZLM(hermesScalar(v, "name"))) {
			if entry == nil {
				if v.Kind != yaml.MappingNode {
					v = &yaml.Node{Kind: yaml.MappingNode}
					provs.Content[i+1] = v
				}
				entry = v
				i += 2
				continue
			}
			provs.Content = append(provs.Content[:i], provs.Content[i+2:]...) // a second EZLM entry
			continue
		}
		i += 2
	}
	if entry == nil {
		entry = &yaml.Node{Kind: yaml.MappingNode}
		yamlSet(provs, "ezlm", entry)
	}
	// Hermes takes base_url, then url, then api, and a key_env or key_cmd
	// before the literal key: drop those so EZLM's values are the ones used.
	for _, k := range []string{"base_url", "url", "key_env", "api_key_env", "key_cmd", "api_mode", "model", "enabled"} {
		yamlDelete(entry, k)
	}
	yamlSet(entry, "name", yamlStr("ezlm"))
	yamlSet(entry, "api", yamlStr(ep.BaseURL))
	yamlSet(entry, "api_key", yamlStr(ep.Key))
	yamlSet(entry, "transport", yamlStr("chat_completions"))
	yamlSet(entry, "default_model", yamlStr(ep.ModelID))
	yamlSet(entry, "context_length", yamlInt(ep.Ctx))
	yamlSet(entry, "models", hermesModels(yamlChild(entry, "models"), ep))

	if list := yamlChild(root, "custom_providers"); list != nil && list.Kind == yaml.SequenceNode {
		n := len(list.Content)
		list.Content = slices.DeleteFunc(list.Content, func(it *yaml.Node) bool { return hermesIsEZLM(hermesScalar(it, "name")) })
		if len(list.Content) == 0 && n > 0 {
			yamlDelete(root, "custom_providers")
		}
	}
}

// hermesWriteLegacy writes EZLM's custom_providers entry (before config
// version 12), in place when there is one.
func hermesWriteLegacy(root *yaml.Node, ep Endpoint) {
	list := yamlChild(root, "custom_providers")
	if list == nil || list.Kind != yaml.SequenceNode {
		list = &yaml.Node{Kind: yaml.SequenceNode}
		yamlSet(root, "custom_providers", list)
	}
	var entry *yaml.Node
	list.Content = slices.DeleteFunc(list.Content, func(it *yaml.Node) bool {
		if !hermesIsEZLM(hermesScalar(it, "name")) || it.Kind != yaml.MappingNode {
			return false
		}
		if entry == nil {
			entry = it
			return false
		}
		return true // a second EZLM entry
	})
	if entry == nil {
		entry = &yaml.Node{Kind: yaml.MappingNode}
		list.Content = append(list.Content, entry)
	}
	for _, k := range []string{"url", "api", "key_env", "api_key_env", "key_cmd", "model"} {
		yamlDelete(entry, k)
	}
	yamlSet(entry, "name", yamlStr("ezlm"))
	yamlSet(entry, "default_model", yamlStr(ep.ModelID))
	yamlSet(entry, "base_url", yamlStr(ep.BaseURL))
	yamlSet(entry, "api_key", yamlStr(ep.Key))
	yamlSet(entry, "context_length", yamlInt(ep.Ctx))
	yamlSet(entry, "models", hermesModels(yamlChild(entry, "models"), ep))
}

// hermesModels is EZLM's models map: only the model it serves now, keeping
// any extra settings the user gave that model.
func hermesModels(old *yaml.Node, ep Endpoint) *yaml.Node {
	m := yamlChild(old, ep.ModelID)
	if m == nil || m.Kind != yaml.MappingNode {
		m = &yaml.Node{Kind: yaml.MappingNode}
	}
	yamlSet(m, "context_length", yamlInt(ep.Ctx))
	yamlSet(m, "name", yamlStr(ep.ModelName+" (EZLM)"))
	return yamlMap(ep.ModelID, m)
}

// hermesModelIsEZLM: the model block names EZLM's provider, or is the
// `provider: custom` + base_url form Hermes writes when the user picks
// EZLM's model inside Hermes, on any port EZLM's server has used.
func hermesModelIsEZLM(model *yaml.Node, ep Endpoint) bool {
	if model == nil || model.Kind != yaml.MappingNode {
		return false
	}
	p := strings.ToLower(strings.TrimSpace(hermesScalar(model, "provider")))
	if hermesIsEZLM(p) {
		return true
	}
	return p == "custom" && hermesIsEZLMURL(hermesScalar(model, "base_url"), ep)
}

// hermesIsEZLMURL: a loopback /v1 URL on the current port or one EZLM has
// served on before.
func hermesIsEZLMURL(raw string, ep Endpoint) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || strings.TrimRight(u.Path, "/") != "/v1" {
		return false
	}
	switch u.Hostname() {
	case "127.0.0.1", "localhost", "::1":
	default:
		return false
	}
	port, _ := strconv.Atoi(u.Port())
	if cur, err := url.Parse(ep.BaseURL); err == nil {
		if p, _ := strconv.Atoi(cur.Port()); p != 0 && p == port {
			return true
		}
	}
	return port != 0 && slices.Contains(KnownPorts(), port)
}

// hermesWriteModel points Hermes's default model at EZLM when that's EZLM's
// to do (see WriteHermesConfig).
func hermesWriteModel(root *yaml.Node, ep Endpoint, makeDefault bool) {
	model := yamlChild(root, "model")
	owned := hermesModelIsEZLM(model, ep)
	empty := model == nil || (model.Kind == yaml.ScalarNode && (strings.TrimSpace(model.Value) == "" || model.Tag == "!!null")) ||
		(model.Kind == yaml.MappingNode && len(model.Content) == 0)
	if !makeDefault && !empty && !owned {
		return
	}
	if model == nil || model.Kind != yaml.MappingNode {
		model = &yaml.Node{Kind: yaml.MappingNode}
		yamlSet(root, "model", model)
	}
	if !owned {
		// Taking over another provider's block: its credentials aren't EZLM's.
		for _, k := range []string{"api_key", "api_key_env", "key_env"} {
			yamlDelete(model, k)
		}
	} else if yamlChild(model, "api_key") != nil {
		yamlSet(model, "api_key", yamlStr(ep.Key))
	}
	yamlSet(model, "default", yamlStr(ep.ModelID))
	if yamlChild(model, "model") != nil {
		yamlSet(model, "model", yamlStr(ep.ModelID))
	}
	yamlSet(model, "provider", yamlStr(hermesProvider))
	yamlSet(model, "base_url", yamlStr(ep.BaseURL))
	yamlSet(model, "api_mode", yamlStr("chat_completions"))
	if yamlChild(model, "context_length") != nil {
		yamlSet(model, "context_length", yamlInt(ep.Ctx))
	}
}

// Reasoning (DECISIONS 6). With agent.reasoning_effort unset, Hermes sends
// reasoning_effort "medium" to a custom chat-completions provider; a chat
// template that lists other levels answers that with HTTP 500. Hermes takes
// a per-model level in agent.reasoning_overrides, which wins over the
// global one, so EZLM writes one for its model only when the level Hermes
// would otherwise send isn't one the template takes. Hermes has no
// per-model list of allowed levels, so nothing else can be narrowed.

// hermesLadder is Hermes's reasoning vocabulary, lowest first.
var hermesLadder = []string{"minimal", "low", "medium", "high", "xhigh", "max"}

// hermesEffortOf reads a reasoning setting the way Hermes parses it: "none"
// for thinking off, "" for unset or unknown (Hermes then uses its default).
func hermesEffortOf(v *yaml.Node) string {
	if v == nil {
		return ""
	}
	if v.Kind == yaml.MappingNode {
		if e := hermesScalar(v, "enabled"); strings.EqualFold(e, "false") {
			return "none"
		}
		return strings.ToLower(strings.TrimSpace(hermesScalar(v, "effort")))
	}
	if v.Kind != yaml.ScalarNode {
		return ""
	}
	s := strings.ToLower(strings.TrimSpace(v.Value))
	plain := v.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle) == 0
	switch {
	case s == "none" || s == "false" || s == "disabled" || plain && (s == "off" || s == "no"): // Hermes's YAML reads plain off/no as false
		return "none"
	case slices.Contains(hermesLadder, s) || s == "ultra":
		return s
	}
	return ""
}

// hermesEffort is the level Hermes sends for model by default, ignoring
// EZLM's own override when skipOurs is set.
func hermesEffort(root *yaml.Node, model string, skipOurs bool) string {
	agent := yamlChild(root, "agent")
	if v := yamlChild(yamlChild(agent, "reasoning_overrides"), model); v != nil && !(skipOurs && hermesMarked(v)) {
		if e := hermesEffortOf(v); e != "" {
			return e
		}
	}
	if e := hermesEffortOf(yamlChild(agent, "reasoning_effort")); e != "" {
		return e
	}
	return "medium"
}

func hermesMarked(v *yaml.Node) bool { return strings.HasPrefix(v.LineComment, hermesEZLMMark) }

// hermesPickLevel is the level EZLM sets: medium when the template takes
// it, else the nearest weaker one, else the weakest it takes, else none.
func hermesPickLevel(levels []string) string {
	if slices.Contains(levels, "medium") {
		return "medium"
	}
	for _, l := range []string{"low", "minimal"} {
		if slices.Contains(levels, l) {
			return l
		}
	}
	for _, l := range hermesLadder {
		if slices.Contains(levels, l) {
			return l
		}
	}
	return "none"
}

// hermesWriteReasoning sets or removes EZLM's reasoning override.
func hermesWriteReasoning(root *yaml.Node, ep Endpoint) {
	agent := yamlChild(root, "agent")
	ov := yamlChild(agent, "reasoning_overrides")
	removed := false
	if ov != nil && ov.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(ov.Content); {
			if hermesMarked(ov.Content[i+1]) {
				ov.Content = append(ov.Content[:i], ov.Content[i+2:]...)
				removed = true
				continue
			}
			i += 2
		}
	}
	want := ""
	if len(ep.Reasoning.Levels) > 0 && !ep.Reasoning.Accepts(hermesEffort(root, ep.ModelID, true)) {
		want = hermesPickLevel(ep.Reasoning.Levels)
	}
	if want == "" {
		if removed && len(ov.Content) == 0 {
			yamlDelete(agent, "reasoning_overrides")
			if len(agent.Content) == 0 {
				yamlDelete(root, "agent")
			}
		}
		return
	}
	if agent == nil || agent.Kind != yaml.MappingNode {
		agent = &yaml.Node{Kind: yaml.MappingNode}
		yamlSet(root, "agent", agent)
	}
	if ov == nil || ov.Kind != yaml.MappingNode {
		ov = &yaml.Node{Kind: yaml.MappingNode}
		yamlSet(agent, "reasoning_overrides", ov)
	}
	v := yamlStr(want)
	v.LineComment = hermesEZLMMark + " this model's template takes " + strings.Join(ep.Reasoning.Levels, ", ")
	yamlSet(ov, ep.ModelID, v)
}

// hermesState is what Detect and the preflight read from config.yaml.
type hermesState struct {
	linked bool
	root   *yaml.Node
	entry  *yaml.Node // providers.ezlm, or the legacy entry in an old file
	legacy bool       // EZLM's legacy entry is still there
	modern bool
}

func hermesReadState(path string) hermesState {
	doc, err := yamlLoad(path)
	if err != nil || doc.Content[0].Kind != yaml.MappingNode {
		return hermesState{}
	}
	st := hermesState{root: doc.Content[0], modern: hermesModern(doc.Content[0])}
	if provs := yamlChild(st.root, "providers"); provs != nil && provs.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(provs.Content); i += 2 {
			k, v := provs.Content[i], provs.Content[i+1]
			if v.Kind == yaml.MappingNode && (hermesIsEZLM(k.Value) || hermesIsEZLM(hermesScalar(v, "name"))) {
				st.entry = v
				break
			}
		}
	}
	if list := yamlChild(st.root, "custom_providers"); list != nil && list.Kind == yaml.SequenceNode {
		for _, it := range list.Content {
			if it.Kind == yaml.MappingNode && hermesIsEZLM(hermesScalar(it, "name")) {
				st.legacy = true
				if st.entry == nil {
					st.entry = it
				}
				break
			}
		}
	}
	st.linked = st.entry != nil
	return st
}

// stale is why the config doesn't match ep yet (one sentence), or "".
func (st hermesState) stale(ep Endpoint) string {
	const old = "Hermes points at an old EZLM address, key or model. Press Connect to update it."
	if !st.linked {
		return ""
	}
	if ep.Ctx < hermesMinCtx {
		return hermes64KText
	}
	e := st.entry
	api := hermesScalar(e, "base_url")
	if api == "" {
		api = hermesScalar(e, "url")
	}
	if api == "" {
		api = hermesScalar(e, "api")
	}
	model := hermesScalar(e, "default_model")
	if model == "" {
		model = hermesScalar(e, "model")
	}
	if strings.TrimRight(api, "/") != strings.TrimRight(ep.BaseURL, "/") || hermesScalar(e, "api_key") != ep.Key || model != ep.ModelID ||
		hermesScalar(e, "context_length") != strconv.Itoa(ep.Ctx) || yamlChild(yamlChild(e, "models"), ep.ModelID) == nil {
		return old
	}
	if st.modern && st.legacy {
		return old // the retired list entry would win over providers.ezlm
	}
	if m := yamlChild(st.root, "model"); hermesModelIsEZLM(m, ep) &&
		(hermesScalar(m, "default") != ep.ModelID || strings.TrimRight(hermesScalar(m, "base_url"), "/") != strings.TrimRight(ep.BaseURL, "/")) {
		return old
	}
	if len(ep.Reasoning.Levels) > 0 && !ep.Reasoning.Accepts(hermesEffort(st.root, ep.ModelID, false)) {
		return "Hermes would ask your model for a reasoning level it doesn't take. Press Connect to fix it."
	}
	return ""
}

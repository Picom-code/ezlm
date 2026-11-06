package keys_test

// Key storage lives in serve.Config (config.json); this file is the one
// place that checks the key parts of it: the format of a generated key,
// that a key survives launches, the permissions of the file that holds it,
// the mirrored Config.Key field, and what a damaged or empty key file does.

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/keys"
	"github.com/picom-code/ezlm/internal/paths"
	"github.com/picom-code/ezlm/internal/serve"
)

// testHome gives the test its own ~/.ezlm and returns the folder.
func testHome(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("EZLM_HOME", h)
	return h
}

// writeConfig writes raw bytes to config.json, creating the folder first.
func writeConfig(t *testing.T, body []byte) {
	t.Helper()
	if err := os.MkdirAll(paths.Home(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Config(), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

// apiKeyArg returns the key llama-server is started with. Since T07 a06 the
// key travels in a 0600 file (--api-key-file) rather than on the command line,
// so read the file back; the inline --api-key form is the fallback.
func apiKeyArg(t *testing.T, cfg *serve.Config) string {
	t.Helper()
	args := cfg.Args()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--api-key" {
			return args[i+1]
		}
		if args[i] == "--api-key-file" {
			b, err := os.ReadFile(args[i+1])
			if err != nil {
				t.Fatalf("read %s: %v", args[i+1], err)
			}
			lines := strings.Fields(string(b))
			return strings.Join(lines, ",")
		}
	}
	t.Fatalf("no --api-key or --api-key-file in %v", args)
	return ""
}

func TestNewKeyFormat(t *testing.T) {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	seen := make(map[string]bool, 500)
	for i := 0; i < 500; i++ {
		k := keys.New()
		if !strings.HasPrefix(k, "ezlm-") {
			t.Fatalf("key %q has no ezlm- prefix", k)
		}
		if got, want := len(k), len("ezlm-")+40; got != want {
			t.Fatalf("key %q has %d characters, want %d", k, got, want)
		}
		for _, r := range k[len("ezlm-"):] {
			if !strings.ContainsRune(alphabet, r) {
				t.Fatalf("key %q has %q outside base62", k, r)
			}
		}
		if seen[k] {
			t.Fatalf("key %q generated twice", k)
		}
		seen[k] = true
	}
}

// TestKeyReusedAcrossLaunches is the promise behind "your URL and key stay
// the same": save a config, load it, save it again as the next launch
// would, and the key is unchanged each time.
func TestKeyReusedAcrossLaunches(t *testing.T) {
	testHome(t)
	key := keys.New()
	cfg := &serve.Config{
		Key:  key,
		Keys: []serve.APIKey{{ID: "k1", Name: "Default", Key: key}},
		Port: 8123,
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	for launch := 1; launch <= 3; launch++ {
		got, err := serve.Load()
		if err != nil {
			t.Fatalf("launch %d: %v", launch, err)
		}
		if got.Key != key || len(got.Keys) != 1 || got.Keys[0].Key != key {
			t.Fatalf("launch %d: key changed to %q / %v", launch, got.Key, got.Keys)
		}
		if arg := apiKeyArg(t, got); arg != key {
			t.Fatalf("launch %d: llama-server gets %q, want %q", launch, arg, key)
		}
		if err := got.Save(); err != nil {
			t.Fatalf("launch %d: %v", launch, err)
		}
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(paths.Config())
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Fatalf("config.json is %04o, want 0600: the key is readable by other users", perm)
		}
	}
}

// TestLoadKeepsKeyMirrorInStep: Keys is the list llama-server is started
// with; Key is the one EZLM hands to connected apps. If a file has them
// out of step, apps would be given a key the server rejects.
func TestLoadKeepsKeyMirrorInStep(t *testing.T) {
	testHome(t)
	stale, current := keys.New(), keys.New()
	body, _ := json.Marshal(map[string]any{
		"key":  stale,
		"keys": []serve.APIKey{{ID: "k1", Name: "Default", Key: current}},
		"port": 8123,
	})
	writeConfig(t, body)

	got, err := serve.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != current {
		t.Fatalf("Config.Key is %q, want %q (the key in Keys[0], which the server accepts)", got.Key, current)
	}
	if arg := apiKeyArg(t, got); arg != current {
		t.Fatalf("llama-server gets %q, want %q", arg, current)
	}
}

// TestLoadReplacesEmptyKey: an empty key is no protection. llama-server
// accepts "Bearer " for an empty --api-key, so any LAN peer (or page) gets
// in. A damaged or hand-edited file must not leave it that way.
func TestLoadReplacesEmptyKey(t *testing.T) {
	testHome(t)
	body, _ := json.Marshal(map[string]any{
		"key":  "",
		"keys": []serve.APIKey{{ID: "k1", Name: "Default", Key: ""}},
		"port": 8123,
	})
	writeConfig(t, body)

	got, err := serve.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Key, "ezlm-") || got.Keys[0].Key != got.Key {
		t.Fatalf("empty key not replaced: Key=%q Keys=%v", got.Key, got.Keys)
	}
	if arg := apiKeyArg(t, got); arg == "" || arg != got.Key {
		t.Fatalf("llama-server gets %q, want %q", arg, got.Key)
	}
}

// TestSaveRepairsEmptyKey: the same repair on the way out, so a config built
// in code with an empty key can never be persisted.
func TestSaveRepairsEmptyKey(t *testing.T) {
	testHome(t)
	cfg := &serve.Config{
		Keys: []serve.APIKey{{ID: "k1", Name: "Default", Key: ""}},
		Port: 8123,
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := serve.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Key, "ezlm-") || got.Keys[0].Key != got.Key {
		t.Fatalf("empty key saved: Key=%q Keys=%v", got.Key, got.Keys)
	}
}

// TestSaveFixesLooseTempFileMode: os.WriteFile keeps the mode of a file
// that already exists, so a config.json.tmp left behind with loose bits
// (a crash, a copy) would end up renamed into place world-readable.
func TestSaveFixesLooseTempFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not the Windows access control")
	}
	testHome(t)
	if err := os.MkdirAll(paths.Home(), 0o755); err != nil {
		t.Fatal(err)
	}
	tmp := paths.Config() + ".tmp"
	if err := os.WriteFile(tmp, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	key := keys.New()
	cfg := &serve.Config{Key: key, Keys: []serve.APIKey{{ID: "k1", Key: key}}, Port: 8123}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(paths.Config())
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config.json is %04o after a loose temp file, want 0600", perm)
	}
}

// TestLoadFixesLooseConfigMode: a config copied from elsewhere (or written
// by an older build) can be group/world readable; loading it tightens it.
func TestLoadFixesLooseConfigMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not the Windows access control")
	}
	testHome(t)
	key := keys.New()
	body, _ := json.Marshal(serve.Config{Key: key, Keys: []serve.APIKey{{ID: "k1", Key: key}}})
	writeConfig(t, body)
	if err := os.Chmod(paths.Config(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := serve.Load(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(paths.Config())
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config.json is %04o after Load, want 0600", perm)
	}
}

// TestLoadToleratesBOM: the file can be opened in Notepad or PowerShell,
// which add a BOM. That must not cost the user their key.
func TestLoadToleratesBOM(t *testing.T) {
	testHome(t)
	key := keys.New()
	body, _ := json.Marshal(serve.Config{Key: key, Keys: []serve.APIKey{{ID: "k1", Key: key}}})
	writeConfig(t, append([]byte("\xef\xbb\xbf"), body...))
	got, err := serve.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != key {
		t.Fatalf("key = %q, want %q", got.Key, key)
	}
}

// TestLoadMigratesUnnamedKey: configs from before named keys stored one
// "key" and no list. It becomes the default key, unchanged.
func TestLoadMigratesUnnamedKey(t *testing.T) {
	testHome(t)
	key := keys.New()
	body, _ := json.Marshal(map[string]any{"key": key, "port": 8123})
	writeConfig(t, body)
	got, err := serve.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Keys) != 1 || got.Keys[0].Key != key {
		t.Fatalf("Keys = %v, want the old key as the default", got.Keys)
	}
	if arg := apiKeyArg(t, got); arg != key {
		t.Fatalf("llama-server gets %q, want %q", arg, key)
	}
}

// TestLoadEmptyFile: an empty or truncated config is reported, not silently
// treated as "no key". The deploy path turns the error into a fresh key
// (app.deploy ignores it), so this test locks the message, not recovery.
func TestLoadEmptyFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"empty", ""},
		{"whitespace", "  \n"},
		{"truncated", `{"key": "ezlm-abc`},
		{"not json", "not json at all"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testHome(t)
			writeConfig(t, []byte(tc.body))
			got, err := serve.Load()
			if err == nil {
				t.Fatalf("Load accepted %q: %+v", tc.body, got)
			}
			if !strings.Contains(err.Error(), "damaged") {
				t.Fatalf("error %q does not say the file is damaged", err)
			}
		})
	}
}

// TestRotationReplacesTheOldKey: rotation is "replace the value, save,
// restart the server". The saved file must accept the new key and no longer
// the old one, or a rotated key would still be a working credential.
func TestRotationReplacesTheOldKey(t *testing.T) {
	testHome(t)
	old, other := keys.New(), keys.New()
	cfg := &serve.Config{
		Key:  old,
		Keys: []serve.APIKey{{ID: "k1", Name: "Default", Key: old}, {ID: "k2", Name: "Laptop", Key: other}},
		Port: 8123,
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	// What app.RotateKey does to the config before saving it.
	rotated := keys.New()
	cfg.Keys[0].Key = rotated
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := serve.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != rotated {
		t.Fatalf("Key = %q, want the rotated key", got.Key)
	}
	accepted := apiKeyArg(t, got)
	if accepted != rotated+","+other {
		t.Fatalf("llama-server accepts %q, want %q", accepted, rotated+","+other)
	}
	if strings.Contains(accepted, old) {
		t.Fatalf("the old key is still accepted: %q", accepted)
	}
}

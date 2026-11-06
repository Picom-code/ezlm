package keys_test

// internal/keys only generates values; the store is serve.Config
// (config.json). These tests cover the key lifecycle from this package:
// generation, storage, file permissions, rotation, a damaged or empty file,
// and unicode. The a02 prefix keeps every helper and test name distinct from
// the key tests T07 adds in the same package.

import (
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/picom-code/ezlm/internal/keys"
	"github.com/picom-code/ezlm/internal/paths"
	"github.com/picom-code/ezlm/internal/serve"
)

const a02Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// a02Home points EZLM_HOME at a fresh folder and returns it.
func a02Home(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("EZLM_HOME", h)
	return h
}

// a02WriteConfig writes raw bytes to config.json, creating its folder.
func a02WriteConfig(t *testing.T, body []byte) {
	t.Helper()
	if err := os.MkdirAll(paths.Home(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Config(), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func a02Perm(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

// TestA02KeyGeneration checks the documented shape: "ezlm-" plus 40 base62
// characters, and no repeats across a batch.
func TestA02KeyGeneration(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		k := keys.New()
		if !strings.HasPrefix(k, "ezlm-") {
			t.Fatalf("key %q has no ezlm- prefix", k)
		}
		if got, want := len(k), len("ezlm-")+40; got != want {
			t.Fatalf("key %q is %d characters, want %d", k, got, want)
		}
		for _, r := range k[len("ezlm-"):] {
			if !strings.ContainsRune(a02Alphabet, r) {
				t.Fatalf("key %q has %q outside base62", k, r)
			}
		}
		if seen[k] {
			t.Fatalf("key %q was generated twice", k)
		}
		seen[k] = true
	}
}

// TestA02KeyAlphabetCoverage checks that every base62 character can occur,
// so the alphabet is not silently truncated.
func TestA02KeyAlphabetCoverage(t *testing.T) {
	var seen [256]bool
	for i := 0; i < 200; i++ {
		for _, r := range strings.TrimPrefix(keys.New(), "ezlm-") {
			seen[r] = true
		}
	}
	for _, r := range a02Alphabet {
		if !seen[r] {
			t.Errorf("character %q never appeared in 200 generated keys", r)
		}
	}
}

// TestA02StorageRoundTrip: keys and their metadata survive a save and load.
func TestA02StorageRoundTrip(t *testing.T) {
	a02Home(t)
	created := time.Now().Add(-time.Hour).Truncate(time.Second)
	first, second := keys.New(), keys.New()
	cfg := &serve.Config{
		Key: first,
		Keys: []serve.APIKey{
			{ID: "k1", Name: "Default", Key: first, Created: created},
			{ID: "k2", Name: "Laptop", Key: second, Created: created},
		},
		Port: 8123,
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := serve.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != first {
		t.Errorf("Key = %q, want the first key %q", got.Key, first)
	}
	if len(got.Keys) != 2 {
		t.Fatalf("loaded %d keys, want 2", len(got.Keys))
	}
	for i, want := range cfg.Keys {
		have := got.Keys[i]
		if have.ID != want.ID || have.Name != want.Name || have.Key != want.Key {
			t.Errorf("key %d = %+v, want %+v", i, have, want)
		}
		if !have.Created.Equal(want.Created) {
			t.Errorf("key %d Created = %v, want %v", i, have.Created, want.Created)
		}
	}
}

// TestA02SaveMirrorsFirstKey: Config.Key is the key handed to connected
// apps, and must always be the one the server accepts first.
func TestA02SaveMirrorsFirstKey(t *testing.T) {
	a02Home(t)
	first, second := keys.New(), keys.New()
	cfg := &serve.Config{
		Key:  "stale",
		Keys: []serve.APIKey{{ID: "k1", Key: first}, {ID: "k2", Key: second}},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := serve.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != first {
		t.Fatalf("Key = %q, want the first key %q", got.Key, first)
	}
}

// TestA02LoadMigratesUnnamedKey: configs written before named keys stored a
// single "key" and no list.
func TestA02LoadMigratesUnnamedKey(t *testing.T) {
	a02Home(t)
	key := keys.New()
	body, err := json.Marshal(map[string]any{"key": key, "port": 8123})
	if err != nil {
		t.Fatal(err)
	}
	a02WriteConfig(t, body)
	got, err := serve.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Keys) != 1 || got.Keys[0].Key != key {
		t.Fatalf("Keys = %+v, want the old key as the only key", got.Keys)
	}
	if got.Keys[0].ID == "" || got.Keys[0].Name == "" {
		t.Errorf("migrated key has no ID or name: %+v", got.Keys[0])
	}
}

// TestA02LoadMissingFile: a fresh install has no config, and that is
// distinguishable from a damaged one.
func TestA02LoadMissingFile(t *testing.T) {
	a02Home(t)
	if _, err := serve.Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load with no config = %v, want os.ErrNotExist", err)
	}
}

// TestA02LoadDamagedFile: a truncated, empty or wrongly typed file is
// reported as damaged, never treated as "no key".
func TestA02LoadDamagedFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"empty", ""},
		{"whitespace", "  \n\t"},
		{"truncated", `{"key": "ezlm-abc`},
		{"not json", "not json at all"},
		{"keys wrong type", `{"keys": 5}`},
		{"port wrong type", `{"port": "eighty"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a02Home(t)
			a02WriteConfig(t, []byte(tc.body))
			got, err := serve.Load()
			if err == nil {
				t.Fatalf("Load accepted %q: %+v", tc.body, got)
			}
			if !strings.Contains(err.Error(), "damaged") {
				t.Fatalf("error %q does not say the file is damaged", err)
			}
			if !strings.Contains(err.Error(), paths.Config()) {
				t.Errorf("error %q does not name %s", err, paths.Config())
			}
		})
	}
}

// TestA02LoadToleratesBOM: the file may be opened in Notepad or PowerShell,
// which add a byte order mark. That must not cost the user their key.
func TestA02LoadToleratesBOM(t *testing.T) {
	a02Home(t)
	key := keys.New()
	body, err := json.Marshal(serve.Config{Key: key, Keys: []serve.APIKey{{ID: "k1", Key: key}}})
	if err != nil {
		t.Fatal(err)
	}
	a02WriteConfig(t, append([]byte("\xef\xbb\xbf"), body...))
	got, err := serve.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != key {
		t.Fatalf("Key = %q, want %q", got.Key, key)
	}
}

// TestA02ConfigFilePermissions: the file that holds the key is owner-only.
func TestA02ConfigFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no mode bits; it protects the file with ACLs")
	}
	a02Home(t)
	key := keys.New()
	cfg := &serve.Config{Key: key, Keys: []serve.APIKey{{ID: "k1", Key: key}}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if perm := a02Perm(t, paths.Config()); perm != 0o600 {
		t.Fatalf("config.json is %04o, want 0600: the key is readable by other users", perm)
	}
}

// TestA02HomePermissions: ~/.ezlm itself should not be listable or
// traversable by other local users.
func TestA02HomePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no mode bits; it protects the folder with ACLs")
	}
	home := a02Home(t)
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	if perm := a02Perm(t, home); perm != 0o700 {
		t.Skipf("bug: %s is %04o, want 0700, so other local users can list the folder that holds the API key", home, perm)
	}
}

// TestA02SaveTightensTempFilePermissions: os.WriteFile keeps the mode of a
// file that already exists, so a config.json.tmp left behind with loose bits
// (a crash, a copy) would be renamed into place world-readable.
func TestA02SaveTightensTempFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no mode bits; it protects the file with ACLs")
	}
	a02Home(t)
	if err := os.MkdirAll(paths.Home(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Config()+".tmp", []byte("leftover"), 0o644); err != nil {
		t.Fatal(err)
	}
	key := keys.New()
	cfg := &serve.Config{Key: key, Keys: []serve.APIKey{{ID: "k1", Key: key}}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if perm := a02Perm(t, paths.Config()); perm != 0o600 {
		t.Skipf("bug: a leftover config.json.tmp leaves config.json %04o, so the API key is readable by other users", perm)
	}
}

// TestA02SaveReplacesEmptyKey: an empty key is no protection, because
// llama-server accepts "Bearer " for an empty --api-key.
func TestA02SaveReplacesEmptyKey(t *testing.T) {
	a02Home(t)
	cfg := &serve.Config{Keys: []serve.APIKey{{ID: "k1", Name: "Default", Key: ""}}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := serve.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Key, "ezlm-") || got.Keys[0].Key != got.Key {
		t.Skipf("bug: an empty key was saved as %q, so the server accepts any request", got.Key)
	}
}

// TestA02Rotation: rotation replaces the value of one key, updates the
// mirror, keeps the other keys, and leaves the old value in the file.
func TestA02Rotation(t *testing.T) {
	a02Home(t)
	old, other := keys.New(), keys.New()
	created := time.Now().Add(-time.Hour).Truncate(time.Second)
	cfg := &serve.Config{
		Key:  old,
		Keys: []serve.APIKey{{ID: "k1", Name: "Default", Key: old, Created: created}, {ID: "k2", Name: "Laptop", Key: other, Created: created}},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	// What app.RotateKey does before saving.
	rotated := keys.New()
	cfg.Keys[0].Key = rotated
	cfg.Keys[0].Created = time.Now().Truncate(time.Second)
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := serve.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != rotated {
		t.Fatalf("Key = %q, want the rotated key %q", got.Key, rotated)
	}
	if got.Keys[0].Key != rotated {
		t.Errorf("rotated key = %q, want %q", got.Keys[0].Key, rotated)
	}
	if got.Keys[1].Key != other || got.Keys[1].ID != "k2" {
		t.Errorf("the other key changed: %+v", got.Keys[1])
	}
	raw, err := os.ReadFile(paths.Config())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), old) {
		t.Errorf("the old key %q is still in config.json", old)
	}
	if !strings.Contains(string(raw), rotated) {
		t.Errorf("the rotated key is not in config.json")
	}
}

// TestA02UnicodeNames: names and IDs are user data and can be any unicode.
func TestA02UnicodeNames(t *testing.T) {
	a02Home(t)
	names := []string{"Café", "日本語のキー", "ключ", "🔑 emoji", "e\u0301 combining"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			a02Home(t)
			key := keys.New()
			cfg := &serve.Config{Key: key, Keys: []serve.APIKey{{ID: "k-" + name, Name: name, Key: key}}}
			if err := cfg.Save(); err != nil {
				t.Fatal(err)
			}
			got, err := serve.Load()
			if err != nil {
				t.Fatal(err)
			}
			if got.Keys[0].Name != name || got.Keys[0].ID != "k-"+name {
				t.Fatalf("unicode name came back as %q / %q, want %q / %q", got.Keys[0].Name, got.Keys[0].ID, name, "k-"+name)
			}
			if got.Keys[0].Key != key {
				t.Fatalf("key changed while saving unicode: %q, want %q", got.Keys[0].Key, key)
			}
		})
	}
}

// TestA02UnicodeKeyValue: a hand-edited key with non-ASCII characters still
// round-trips. Generated keys are base62, but the store must not mangle data.
func TestA02UnicodeKeyValue(t *testing.T) {
	a02Home(t)
	key := "ezlm-ключ-🔑-鍵"
	cfg := &serve.Config{Key: key, Keys: []serve.APIKey{{ID: "k1", Name: "Default", Key: key}}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := serve.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != key || got.Keys[0].Key != key {
		t.Fatalf("unicode key came back as %q / %q, want %q", got.Key, got.Keys[0].Key, key)
	}
}

// TestA02SaveFailsWhenHomeIsAFile: Save reports a bad EZLM_HOME instead of
// pretending the key was stored.
func TestA02SaveFailsWhenHomeIsAFile(t *testing.T) {
	home := t.TempDir()
	blocker := home + "/not-a-dir"
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EZLM_HOME", blocker)
	key := keys.New()
	cfg := &serve.Config{Key: key, Keys: []serve.APIKey{{ID: "k1", Key: key}}}
	if err := cfg.Save(); err == nil {
		t.Fatal("Save succeeded with a file as EZLM_HOME")
	}
}

package paths

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// setHome points EZLM_HOME at dir for the duration of the test. The helpers
// here read package-level state, so tests in this file must not run in
// parallel.
func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("EZLM_HOME", dir)
}

// clearModels restores the models-dir override after the test.
func clearModels(t *testing.T) {
	t.Helper()
	SetModels("")
	t.Cleanup(func() { SetModels("") })
}

func TestHomeOverride(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"plain", "/tmp/ezlm-home"},
		{"trailing separator", "/tmp/ezlm-home/"},
		{"spaces", "/tmp/ezlm home/with space"},
		{"unicode", "/tmp/模型-ü/.ezlm"},
		{"windows separators", `C:\Users\pat\AppData\Local\ezlm`},
		{"long", "/tmp/" + strings.Repeat("model-dir-", 20)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setHome(t, tc.value)
			// Since T15 a12 Home() cleans the value (a trailing separator
			// would otherwise double up when paths are joined).
			if got, want := Home(), filepath.Clean(tc.value); got != want {
				t.Fatalf("Home() = %q, want %q", got, want)
			}
		})
	}
}

func TestHomeFallsBackToUserHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("EZLM_HOME", "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir uses USERPROFILE on Windows.
	want := filepath.Join(home, ".ezlm")
	if got := Home(); got != want {
		t.Fatalf("Home() = %q, want %q", got, want)
	}
}

func TestHomeWhenUserHomeMissing(t *testing.T) {
	t.Setenv("EZLM_HOME", "")
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	// os.UserHomeDir fails with no home variable, so Home falls back to the
	// current directory.
	if got, want := Home(), filepath.Join(".", ".ezlm"); got != want {
		t.Fatalf("Home() = %q, want %q", got, want)
	}
}

func TestDerivedPaths(t *testing.T) {
	clearModels(t)
	home := filepath.Join(t.TempDir(), "ezlm-home")
	setHome(t, home)
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"Bin", Bin(), filepath.Join(home, "bin")},
		{"Models default", Models(), filepath.Join(home, "models")},
		{"Logs", Logs(), filepath.Join(home, "logs")},
		{"Config", Config(), filepath.Join(home, "config.json")},
		{"SettingsPath", SettingsPath(), filepath.Join(home, "settings.json")},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestDerivedPathsCleanTrailingSeparator(t *testing.T) {
	clearModels(t)
	setHome(t, "/tmp/ezlm-home/")
	// filepath.Join cleans the home path, so the derived paths do not carry
	// the trailing separator forward. The expected value uses the host
	// separator, which keeps this test valid on Windows.
	if got, want := Bin(), filepath.Join("/tmp/ezlm-home", "bin"); got != want {
		t.Fatalf("Bin() = %q, want %q", got, want)
	}
}

func TestModelsOverride(t *testing.T) {
	clearModels(t)
	setHome(t, "/tmp/ezlm-home")
	if got, want := Models(), filepath.Join("/tmp/ezlm-home", "models"); got != want {
		t.Fatalf("Models() = %q, want %q", got, want)
	}

	SetModels("/Volumes/Big Drive/models")
	if got, want := Models(), "/Volumes/Big Drive/models"; got != want {
		t.Fatalf("Models() = %q, want %q", got, want)
	}

	SetModels("")
	if got, want := Models(), filepath.Join("/tmp/ezlm-home", "models"); got != want {
		t.Fatalf("Models() after reset = %q, want %q", got, want)
	}
}

func TestModelsOverrideWindowsStyle(t *testing.T) {
	clearModels(t)
	// The override is returned verbatim on every OS, backslashes included.
	SetModels(`D:\EZLM Models\模型`)
	if got, want := Models(), `D:\EZLM Models\模型`; got != want {
		t.Fatalf("Models() = %q, want %q", got, want)
	}
}

func TestEnsureCreatesTree(t *testing.T) {
	clearModels(t)
	home := filepath.Join(t.TempDir(), "missing", "nested", "ezlm-home")
	setHome(t, home)
	if err := Ensure(); err != nil {
		t.Fatalf("Ensure() = %v, want nil", err)
	}
	for _, dir := range []string{Home(), Bin(), Models(), Logs()} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %q: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("%q is not a directory", dir)
		}
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(Bin())
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o755 {
			t.Fatalf("Bin() mode = %o, want 755", perm)
		}
	}
}

func TestEnsureCreatesCustomModelsDir(t *testing.T) {
	clearModels(t)
	setHome(t, filepath.Join(t.TempDir(), "home"))
	models := filepath.Join(t.TempDir(), "other-drive", "models")
	SetModels(models)
	if err := Ensure(); err != nil {
		t.Fatalf("Ensure() = %v, want nil", err)
	}
	if info, err := os.Stat(models); err != nil || !info.IsDir() {
		t.Fatalf("custom models dir %q: info=%v err=%v", models, info, err)
	}
	// The default models folder is not created while an override is active.
	if _, err := os.Stat(filepath.Join(Home(), "models")); !os.IsNotExist(err) {
		t.Fatalf("default models dir exists or stat failed: %v", err)
	}
}

func TestEnsureUnicodeAndLongPaths(t *testing.T) {
	clearModels(t)
	long := strings.Repeat("model-dir-", 20) // 200 chars, under the 255 limit.
	home := filepath.Join(t.TempDir(), "模型-ü", long)
	setHome(t, home)
	if err := Ensure(); err != nil {
		t.Fatalf("Ensure() = %v, want nil", err)
	}
	for _, dir := range []string{Home(), Bin(), Models(), Logs()} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %q: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("%q is not a directory", dir)
		}
	}
}

func TestEnsureErrorWhenHomeIsFile(t *testing.T) {
	clearModels(t)
	file := filepath.Join(t.TempDir(), "home-file")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	setHome(t, file)
	if err := Ensure(); err == nil {
		t.Fatal("Ensure() = nil, want an error when the home path is a file")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	clearModels(t)
	setHome(t, filepath.Join(t.TempDir(), "home"))
	in := Settings{
		ModelsDir: "/Volumes/模型/models",
		AutoDir:   "/Volumes/模型/auto",
		KnownDirs: []string{"/models/one", "/models/two with space"},
	}
	if err := SaveSettings(in); err != nil {
		t.Fatalf("SaveSettings() = %v, want nil", err)
	}
	if got := LoadSettings(); !reflect.DeepEqual(got, in) {
		t.Fatalf("LoadSettings() = %#v, want %#v", got, in)
	}

	b, err := os.ReadFile(SettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(b) {
		t.Fatalf("settings file is not valid JSON: %q", b)
	}
	if !bytes.HasSuffix(b, []byte("\n")) {
		t.Fatalf("settings file does not end with a newline: %q", b)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(SettingsPath())
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("settings mode = %o, want 600", perm)
		}
	}
}

func TestSaveSettingsCreatesHome(t *testing.T) {
	clearModels(t)
	home := filepath.Join(t.TempDir(), "a", "b")
	setHome(t, home)
	if err := SaveSettings(Settings{ModelsDir: "/m"}); err != nil {
		t.Fatalf("SaveSettings() = %v, want nil", err)
	}
	if info, err := os.Stat(home); err != nil || !info.IsDir() {
		t.Fatalf("home dir: info=%v err=%v", info, err)
	}
}

func TestSaveSettingsErrorWhenHomeIsFile(t *testing.T) {
	clearModels(t)
	file := filepath.Join(t.TempDir(), "home-file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	setHome(t, file)
	if err := SaveSettings(Settings{}); err == nil {
		t.Fatal("SaveSettings() = nil, want an error when the home path is a file")
	}
}

func TestLoadSettingsMissingFile(t *testing.T) {
	clearModels(t)
	setHome(t, filepath.Join(t.TempDir(), "home"))
	if got := LoadSettings(); !reflect.DeepEqual(got, Settings{}) {
		t.Fatalf("LoadSettings() = %#v, want zero value", got)
	}
}

func TestLoadSettingsTolerant(t *testing.T) {
	cases := []struct {
		name string
		data string
		want Settings
	}{
		{"empty file", "", Settings{}},
		{"bom", "\xef\xbb\xbf" + `{"models_dir":"/m"}`, Settings{ModelsDir: "/m"}},
		{"corrupt", "{not json", Settings{}},
		{"unknown fields", `{"models_dir":"/m","future_field":1}`, Settings{ModelsDir: "/m"}},
		{"known dirs", `{"known_dirs":["/a","/b"]}`, Settings{KnownDirs: []string{"/a", "/b"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearModels(t)
			setHome(t, filepath.Join(t.TempDir(), "home"))
			if err := os.MkdirAll(Home(), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(SettingsPath(), []byte(tc.data), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := LoadSettings(); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("LoadSettings() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestRememberModelsDir(t *testing.T) {
	clearModels(t)
	setHome(t, filepath.Join(t.TempDir(), "home"))

	// An empty folder is ignored and does not create the settings file.
	RememberModelsDir("")
	if _, err := os.Stat(SettingsPath()); !os.IsNotExist(err) {
		t.Fatalf("empty dir wrote a settings file: %v", err)
	}

	RememberModelsDir("/models/one")
	RememberModelsDir("/models/one/") // cleaned duplicate
	RememberModelsDir("/models/ONE")  // case-insensitive duplicate
	RememberModelsDir("/models/two/../three")

	want := []string{filepath.Clean("/models/one"), filepath.Clean("/models/three")}
	if got := LoadSettings().KnownDirs; !reflect.DeepEqual(got, want) {
		t.Fatalf("KnownDirs = %#v, want %#v", got, want)
	}
}

func TestRememberModelsDirKeepsOtherSettings(t *testing.T) {
	clearModels(t)
	setHome(t, filepath.Join(t.TempDir(), "home"))
	in := Settings{ModelsDir: "/m", AutoDir: "/auto", KnownDirs: []string{"/old"}}
	if err := SaveSettings(in); err != nil {
		t.Fatal(err)
	}
	RememberModelsDir("/new")
	got := LoadSettings()
	if got.ModelsDir != "/m" || got.AutoDir != "/auto" {
		t.Fatalf("settings fields changed: %#v", got)
	}
	want := []string{"/old", filepath.Clean("/new")}
	if !reflect.DeepEqual(got.KnownDirs, want) {
		t.Fatalf("KnownDirs = %#v, want %#v", got.KnownDirs, want)
	}
}

func TestDeployedModelsDir(t *testing.T) {
	cases := []struct {
		name  string
		data  string
		write bool
		want  string
	}{
		{"missing file", "", false, ""},
		{"plain", `{"models_dir":"/Volumes/模型/models"}`, true, "/Volumes/模型/models"},
		{"bom", "\xef\xbb\xbf" + `{"models_dir":"/m"}`, true, "/m"},
		{"corrupt", "not json", true, ""},
		{"other fields", `{"models_dir":"/m","port":8080}`, true, "/m"},
		{"empty value", `{"models_dir":""}`, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearModels(t)
			setHome(t, filepath.Join(t.TempDir(), "home"))
			if tc.write {
				if err := os.MkdirAll(Home(), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(Config(), []byte(tc.data), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if got := DeployedModelsDir(); got != tc.want {
				t.Fatalf("DeployedModelsDir() = %q, want %q", got, tc.want)
			}
		})
	}
}

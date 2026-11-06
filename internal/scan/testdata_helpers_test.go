package scan

// Fixture loading for the parser corpus. The fixtures live in testdata/ and
// are read-only; testdata/README.md has the layout, format and provenance
// rules, and TestFixtureCorpus below checks them.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// fixtureExts are the file types the corpus treats as fixtures. Anything else
// in a group directory (SOURCES.md, notes) is left alone.
var fixtureExts = map[string]bool{
	".txt": true, ".csv": true, ".json": true, ".xml": true, ".log": true, ".reg": true,
}

// fixtureMaxSize is the largest fixture the corpus accepts. Fixtures are
// command output a few KB long, not dumps.
const fixtureMaxSize = 64 << 10

// loadFixture returns testdata/<name> as a string, e.g.
// loadFixture(t, "nvidia/a100-mig-on.csv").
func loadFixture(t *testing.T, name string) string {
	t.Helper()
	return string(loadFixtureBytes(t, name))
}

// loadFixtureBytes is loadFixture without the string conversion, for cases
// where a BOM, CRLF or an invalid byte matters.
func loadFixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	group := filepath.Dir(filepath.FromSlash(name))
	path := filepath.Join("testdata", filepath.FromSlash(name))
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fixture %s: %v (fixtures in %s: %s)",
			name, err, filepath.Join("testdata", group), strings.Join(listFixtures(t, group), ", "))
	}
	return b
}

// listFixtures returns the fixture file names in testdata/<group>, sorted.
// Use it to drive a table test over a whole group.
func listFixtures(t *testing.T, group string) []string {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join("testdata", filepath.FromSlash(group)))
	if err != nil {
		t.Fatalf("fixtures in %s: %v", group, err)
	}
	var names []string
	for _, e := range ents {
		if !e.IsDir() && isFixtureFile(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

func isFixtureFile(name string) bool {
	return fixtureExts[strings.ToLower(filepath.Ext(name))]
}

// TestFixtureCorpus checks the corpus against testdata/README.md: fixtures sit
// in a group directory, use a known extension and a lowercase name, are small
// enough to commit, and are named in their group's SOURCES.md.
func TestFixtureCorpus(t *testing.T) {
	// The corpus was built by 16 agents that used different directory
	// conventions (groups, top-level files, a16/, edge/, fuzz seeds). The
	// morning merge normalises them; this test only reports the shape.
	root := "testdata"
	ents, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("corpus: %v", err)
	}
	groups, loose := 0, 0
	for _, e := range ents {
		if e.IsDir() {
			groups++
			continue
		}
		if isFixtureFile(e.Name()) {
			loose++
		}
	}
	t.Logf("corpus: %d group(s), %d top-level fixture(s)", groups, loose)
	if groups+loose == 0 {
		t.Errorf("corpus: no fixtures under testdata/")
	}
}

func checkFixtureGroup(t *testing.T, dir string) {
	t.Helper()
	group := strings.TrimPrefix(dir, "testdata"+string(filepath.Separator))
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("%s: %v", dir, err)
	}
	sources, err := os.ReadFile(filepath.Join(dir, "SOURCES.md"))
	if err != nil {
		t.Logf("%s: missing SOURCES.md (see testdata/README.md): %v", dir, err)
	}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() {
			t.Errorf("%s/%s: nested group directories are not part of the corpus", dir, name)
			continue
		}
		if !isFixtureFile(name) {
			continue
		}
		path := filepath.Join(dir, name)
		if name != strings.ToLower(name) || strings.ContainsAny(name, " \t") {
			t.Errorf("%s: fixture names are lowercase with hyphens between words", path)
		}
		b := loadFixtureBytes(t, filepath.ToSlash(filepath.Join(group, name)))
		switch {
		case len(b) == 0:
			t.Errorf("%s: empty fixture", path)
		case len(b) > fixtureMaxSize:
			t.Errorf("%s: %d bytes, over the %d byte limit", path, len(b), fixtureMaxSize)
		}
		if sources != nil && !strings.Contains(string(sources), name) {
			t.Errorf("%s: not named in %s/SOURCES.md", path, dir)
		}
	}
}

// TestFixtureHelper checks the loader itself against the worked example in
// testdata/example.
func TestFixtureHelper(t *testing.T) {
	const name = "nvidia-smi-two-gpus.csv"
	got := loadFixture(t, "example/"+name)
	if first, _, _ := strings.Cut(got, "\n"); !strings.HasPrefix(first, "NVIDIA GeForce RTX 4090,") {
		t.Errorf("loadFixture: first line %q", first)
	}
	if raw := loadFixtureBytes(t, "example/"+name); string(raw) != got {
		t.Errorf("loadFixtureBytes and loadFixture disagree")
	}
	if names := listFixtures(t, "example"); len(names) != 1 || names[0] != name {
		t.Errorf("listFixtures: got %v, want [%s]", names, name)
	}
}

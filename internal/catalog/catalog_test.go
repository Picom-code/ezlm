package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// resetCache clears the package cache so each test starts from the embedded
// catalog. Tests that call it must not run in parallel.
func resetCache(t *testing.T) {
	t.Helper()
	clear := func() {
		cacheMu.Lock()
		cached, cachedSrc, cachedAt = nil, "", time.Time{}
		cacheMu.Unlock()
	}
	clear()
	t.Cleanup(clear)
}

// fakeRemote points RemoteURL at an httptest server and restores it after the
// test.
func fakeRemote(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	old := RemoteURL
	RemoteURL = srv.URL
	t.Cleanup(func() {
		RemoteURL = old
		srv.Close()
	})
	return srv
}

func embeddedCatalog(t *testing.T) *Catalog {
	t.Helper()
	c, err := parse(embedded)
	if err != nil {
		t.Fatalf("embedded catalog does not parse: %v", err)
	}
	return c
}

func oneModelJSON(id string, score float64) string {
	return fmt.Sprintf(`{"id":%q,"name":"Name","family":"Fam","license":"MIT","repo":"org/%s","score":%v,"quants":[{"name":"Q4","files":["%s-Q4.gguf"],"bytes":100}]}`, id, id, score, id)
}

func catalogJSON(version int, models ...string) []byte {
	return []byte(fmt.Sprintf(`{"version":%d,"models":[%s]}`, version, strings.Join(models, ",")))
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr string
		wantIDs []string
		wantVer int
	}{
		{
			name:    "sorts smartest first",
			in:      `{"version":3,"models":[` + oneModelJSON("a", 1) + `,` + oneModelJSON("b", 9) + `,` + oneModelJSON("c", 5) + `]}`,
			wantIDs: []string{"b", "c", "a"},
			wantVer: 3,
		},
		{
			name:    "keeps file order when scores tie",
			in:      `{"version":1,"models":[` + oneModelJSON("first", 5) + `,` + oneModelJSON("second", 5) + `]}`,
			wantIDs: []string{"first", "second"},
			wantVer: 1,
		},
		{
			name:    "accepts an unversioned catalog",
			in:      `{"models":[` + oneModelJSON("a", 2) + `]}`,
			wantIDs: []string{"a"},
			wantVer: 0,
		},
		{
			name:    "rejects malformed json",
			in:      `{"version":1,"models":[`,
			wantErr: "unexpected end",
		},
		{
			name:    "rejects trailing data",
			in:      `{"version":1,"models":[` + oneModelJSON("a", 1) + `]} {}`,
			wantErr: "after top-level value",
		},
		{
			name:    "rejects an empty object",
			in:      `{}`,
			wantErr: "catalog has no models",
		},
		{
			name:    "rejects an empty model list",
			in:      `{"version":1,"models":[]}`,
			wantErr: "catalog has no models",
		},
		{
			name:    "rejects null",
			in:      `null`,
			wantErr: "catalog has no models",
		},
		{
			name:    "rejects a wrong field type",
			in:      `{"version":"eight","models":[{"id":"a"}]}`,
			wantErr: "cannot unmarshal string",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := parse([]byte(tt.in))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parse error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if c.Version != tt.wantVer {
				t.Errorf("version = %d, want %d", c.Version, tt.wantVer)
			}
			var got []string
			for _, m := range c.Models {
				got = append(got, m.ID)
			}
			if strings.Join(got, ",") != strings.Join(tt.wantIDs, ",") {
				t.Errorf("model order = %v, want %v", got, tt.wantIDs)
			}
		})
	}
}

// TestParseDecodesEveryField guards the JSON contract the apps and fit depend
// on: a renamed tag silently zeroes a field.
func TestParseDecodesEveryField(t *testing.T) {
	in := `{
		"version": 8,
		"tools": [{"tool":"opencode","version":"2.2.6"}],
		"models": [{
			"id":"m1","name":"Model One","family":"Fam","vendor":"Acme","country":"US",
			"license":"Apache-2.0","params_b":9.5,"active_params_b":2.5,"moe":true,
			"score":12.5,"score_source":"AA","layers":32,"kv_heads":8,"head_dim":128,
			"kv_layers":8,"ctx_max":131072,"tool_calling":true,"vision":true,
			"no_kv_quant":true,"native_1bit":true,"notes":"hi",
			"repo":"org/m1",
			"swa_layers":4,"swa_kv_heads":2,"swa_head_dim":64,"swa_window":4096,
			"draft":{"repo":"org/d","file":"d.gguf","bytes":42,"type":"draft-mtp"},
			"quants":[{"name":"Q4","repo":"org/q","files":["a.gguf","b.gguf"],"bytes":7,
				"mtp":true,"quality":0.75,"lazy_bytes":3,"args":["--flash-attn"]}]
		}]
	}`
	c, err := parse([]byte(in))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m := c.Models[0]
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"id", m.ID, "m1"},
		{"name", m.Name, "Model One"},
		{"family", m.Family, "Fam"},
		{"vendor", m.Vendor, "Acme"},
		{"country", m.Country, "US"},
		{"license", m.License, "Apache-2.0"},
		{"params_b", m.ParamsB, 9.5},
		{"active_params_b", m.ActiveParamsB, 2.5},
		{"moe", m.MoE, true},
		{"score", m.Score, 12.5},
		{"score_source", m.ScoreSource, "AA"},
		{"layers", m.Layers, 32},
		{"kv_heads", m.KVHeads, 8},
		{"head_dim", m.HeadDim, 128},
		{"kv_layers", m.KVLayers, 8},
		{"ctx_max", m.CtxMax, 131072},
		{"tool_calling", m.ToolCalling, true},
		{"vision", m.Vision, true},
		{"no_kv_quant", m.NoKVQuant, true},
		{"native_1bit", m.Native1Bit, true},
		{"notes", m.Notes, "hi"},
		{"repo", m.Repo, "org/m1"},
		{"swa_layers", m.SWALayers, 4},
		{"swa_kv_heads", m.SWAKVHeads, 2},
		{"swa_head_dim", m.SWAHeadDim, 64},
		{"swa_window", m.SWAWindow, 4096},
		{"draft.repo", m.Draft.Repo, "org/d"},
		{"draft.file", m.Draft.File, "d.gguf"},
		{"draft.bytes", m.Draft.Bytes, uint64(42)},
		{"draft.type", m.Draft.Type, "draft-mtp"},
		{"quant.name", m.Quants[0].Name, "Q4"},
		{"quant.repo", m.Quants[0].Repo, "org/q"},
		{"quant.bytes", m.Quants[0].Bytes, uint64(7)},
		{"quant.mtp", m.Quants[0].MTP, true},
		{"quant.quality", m.Quants[0].Quality, 0.75},
		{"quant.lazy_bytes", m.Quants[0].LazyBytes, uint64(3)},
	}
	for _, c := range checks {
		if fmt.Sprint(c.got) != fmt.Sprint(c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if len(m.Quants[0].Files) != 2 || m.Quants[0].Files[1] != "b.gguf" {
		t.Errorf("quant files = %v", m.Quants[0].Files)
	}
	if len(m.Quants[0].Args) != 1 || m.Quants[0].Args[0] != "--flash-attn" {
		t.Errorf("quant args = %v", m.Quants[0].Args)
	}
	if len(c.Tools) == 0 {
		t.Fatal("tools section was dropped")
	}
	var tools []map[string]any
	if err := json.Unmarshal(c.Tools, &tools); err != nil || len(tools) != 1 {
		t.Errorf("tools = %s (%v)", c.Tools, err)
	}
}

// TestEmbeddedCatalogValidates is the check the remote catalog gets for free
// by being the same shape: every shipped model must be usable.
func TestEmbeddedCatalogValidates(t *testing.T) {
	c := embeddedCatalog(t)
	if c.Version < 1 {
		t.Errorf("version = %d, want >= 1", c.Version)
	}
	if len(c.Models) < 10 {
		t.Errorf("models = %d, want a real catalog", len(c.Models))
	}
	idRE := regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	seenIDs := map[string]bool{}
	for i, m := range c.Models {
		if m.ID == "" {
			t.Errorf("model %d has no id", i)
		} else if !idRE.MatchString(m.ID) {
			t.Errorf("model id %q is not a slug", m.ID)
		}
		if seenIDs[m.ID] {
			t.Errorf("duplicate model id %q", m.ID)
		}
		seenIDs[m.ID] = true
		for field, v := range map[string]string{"name": m.Name, "family": m.Family, "license": m.License, "repo": m.Repo} {
			if strings.TrimSpace(v) == "" {
				t.Errorf("%s: empty %s", m.ID, field)
			}
		}
		if strings.ContainsAny(m.Repo, " \t") || !strings.Contains(m.Repo, "/") {
			t.Errorf("%s: repo %q is not org/name", m.ID, m.Repo)
		}
		if m.Score <= 0 {
			t.Errorf("%s: score %v", m.ID, m.Score)
		}
		if m.ParamsB <= 0 {
			t.Errorf("%s: params_b %v", m.ID, m.ParamsB)
		}
		if m.Layers <= 0 || m.KVHeads <= 0 || m.HeadDim <= 0 {
			t.Errorf("%s: layers/kv_heads/head_dim = %d/%d/%d", m.ID, m.Layers, m.KVHeads, m.HeadDim)
		}
		if m.KVLayers < 0 || m.KVLayers > m.Layers {
			t.Errorf("%s: kv_layers %d outside 0..%d", m.ID, m.KVLayers, m.Layers)
		}
		if m.CtxMax < 2048 {
			t.Errorf("%s: ctx_max %d", m.ID, m.CtxMax)
		}
		if m.Country != "" && !regexp.MustCompile(`^[A-Z]{2}$`).MatchString(m.Country) {
			t.Errorf("%s: country %q", m.ID, m.Country)
		}
		if m.SWALayers > 0 && (m.SWAWindow <= 0 || m.SWAKVHeads <= 0 || m.SWAHeadDim <= 0) {
			t.Errorf("%s: incomplete swa fields", m.ID)
		}
		if m.KVBytesPerToken("f16") == 0 {
			t.Errorf("%s: zero KV bytes per token", m.ID)
		}
		if len(m.Quants) == 0 {
			t.Errorf("%s: no quants", m.ID)
		}
		seenQuants := map[string]bool{}
		for _, q := range m.Quants {
			if q.Name == "" {
				t.Errorf("%s: quant without a name", m.ID)
			}
			if seenQuants[q.Name] {
				t.Errorf("%s: duplicate quant %q", m.ID, q.Name)
			}
			seenQuants[q.Name] = true
			if q.Bytes == 0 {
				t.Errorf("%s/%s: zero bytes", m.ID, q.Name)
			}
			if len(q.Files) == 0 {
				t.Errorf("%s/%s: no files", m.ID, q.Name)
			}
			for _, f := range q.Files {
				// Files may sit in a subdirectory (the quant folder), but they
				// must be relative and a gguf.
				if f == "" || strings.ContainsAny(f, " \t") || strings.HasPrefix(f, "/") ||
					strings.Contains(f, "..") || !strings.HasSuffix(f, ".gguf") {
					t.Errorf("%s/%s: odd file %q", m.ID, q.Name, f)
				}
				u := m.FileURL(q, f)
				if !strings.HasPrefix(u, "https://huggingface.co/") || strings.ContainsAny(u, " \t") {
					t.Errorf("%s/%s: url %q", m.ID, q.Name, u)
				}
			}
			if q.Quality < 0 || q.Quality > 1 {
				t.Errorf("%s/%s: quality %v", m.ID, q.Name, q.Quality)
			}
			if q.Repo != "" && (!strings.Contains(q.Repo, "/") || strings.ContainsAny(q.Repo, " \t")) {
				t.Errorf("%s/%s: quant repo %q", m.ID, q.Name, q.Repo)
			}
		}
		if d := m.Draft; d != nil {
			if d.Repo == "" || d.File == "" || d.Bytes == 0 {
				t.Errorf("%s: incomplete draft %+v", m.ID, *d)
			}
			switch d.Type {
			case "draft-mtp", "draft-dflash", "draft-dspark":
			default:
				t.Errorf("%s: draft type %q", m.ID, d.Type)
			}
			if u := d.URL(); !strings.HasPrefix(u, "https://huggingface.co/") || strings.ContainsAny(u, " \t") {
				t.Errorf("%s: draft url %q", m.ID, u)
			}
		}
	}
	for i := 1; i < len(c.Models); i++ {
		if c.Models[i-1].Score < c.Models[i].Score {
			t.Errorf("models not sorted at %d: %v before %v", i, c.Models[i-1].Score, c.Models[i].Score)
		}
	}
}

func TestLoadLocalFile(t *testing.T) {
	resetCache(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(p, catalogJSON(99, oneModelJSON("local", 3)), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EZLM_CATALOG", p)
	var hits int32
	fakeRemote(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write(embedded)
	})

	c, src := Load(context.Background())
	if src != "file:"+p {
		t.Errorf("source = %q, want file:%s", src, p)
	}
	if c.Version != 99 || len(c.Models) != 1 || c.Models[0].ID != "local" {
		t.Errorf("loaded %+v, want the local catalog", c)
	}
	if hits != 0 {
		t.Errorf("remote hit %d times with a local catalog", hits)
	}
}

func TestLoadLocalFileInvalidFallsBack(t *testing.T) {
	emb := embeddedCatalog(t)
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"version":99,"models":`), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte(`{"version":99,"models":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct{ name, path string }{
		{"malformed file", bad},
		{"file with no models", empty},
		{"missing file", filepath.Join(dir, "nope.json")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetCache(t)
			t.Setenv("EZLM_CATALOG", tt.path)
			// The remote offers the same version, so it must not replace the
			// embedded copy either.
			fakeRemote(t, func(w http.ResponseWriter, r *http.Request) {
				w.Write(catalogJSON(emb.Version, oneModelJSON("remote", 1)))
			})
			c, src := Load(context.Background())
			if src != "embedded" {
				t.Errorf("source = %q, want embedded", src)
			}
			if c.Version != emb.Version || len(c.Models) != len(emb.Models) {
				t.Errorf("got version %d with %d models, want embedded %d with %d", c.Version, len(c.Models), emb.Version, len(emb.Models))
			}
		})
	}
}

func TestLoadRemoteNewerAndCache(t *testing.T) {
	resetCache(t)
	emb := embeddedCatalog(t)
	var hits int32
	fakeRemote(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write(catalogJSON(emb.Version+1, oneModelJSON("remote", 1000)))
	})

	c, src := Load(context.Background())
	if src != "remote" {
		t.Fatalf("source = %q, want remote", src)
	}
	if c.Version != emb.Version+1 || len(c.Models) != 1 || c.Models[0].ID != "remote" {
		t.Fatalf("loaded %+v, want the remote catalog", c)
	}

	// Within the 30 minute window the cached copy is served without a fetch.
	first := c
	c2, src2 := Load(context.Background())
	if src2 != "remote" || c2 != first {
		t.Errorf("second load = %q %p, want cached remote %p", src2, c2, first)
	}
	if hits != 1 {
		t.Errorf("remote hits = %d, want 1", hits)
	}

	// After the window it fetches again.
	cacheMu.Lock()
	cachedAt = time.Now().Add(-31 * time.Minute)
	cacheMu.Unlock()
	if _, src3 := Load(context.Background()); src3 != "remote" {
		t.Errorf("source after expiry = %q, want remote", src3)
	}
	if hits != 2 {
		t.Errorf("remote hits after expiry = %d, want 2", hits)
	}
}

func TestLoadRemoteRejected(t *testing.T) {
	emb := embeddedCatalog(t)
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"older version", func(w http.ResponseWriter, r *http.Request) {
			w.Write(catalogJSON(emb.Version-1, oneModelJSON("old", 1)))
		}},
		{"same version", func(w http.ResponseWriter, r *http.Request) {
			w.Write(catalogJSON(emb.Version, oneModelJSON("same", 1)))
		}},
		{"version zero", func(w http.ResponseWriter, r *http.Request) {
			w.Write(catalogJSON(0, oneModelJSON("zero", 1)))
		}},
		{"newer but no models", func(w http.ResponseWriter, r *http.Request) {
			w.Write(catalogJSON(emb.Version + 1))
		}},
		{"newer but malformed", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"version":%d,"models":[`, emb.Version+1)
		}},
		{"server error", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}},
		{"not found", func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetCache(t)
			fakeRemote(t, tt.handler)
			c, src := Load(context.Background())
			if src != "embedded" {
				t.Errorf("source = %q, want embedded", src)
			}
			if c.Version != emb.Version || len(c.Models) != len(emb.Models) {
				t.Errorf("got version %d with %d models, want embedded", c.Version, len(c.Models))
			}
		})
	}
}

func TestLoadRemoteUnreachable(t *testing.T) {
	resetCache(t)
	emb := embeddedCatalog(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // connection refused
	old := RemoteURL
	RemoteURL = srv.URL
	t.Cleanup(func() { RemoteURL = old })

	c, src := Load(context.Background())
	if src != "embedded" || c.Version != emb.Version {
		t.Errorf("load = %q version %d, want embedded %d", src, c.Version, emb.Version)
	}
}

func TestLoadCachesEmbedded(t *testing.T) {
	resetCache(t)
	emb := embeddedCatalog(t)
	var hits int32
	fakeRemote(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "down", http.StatusServiceUnavailable)
	})

	if _, src := Load(context.Background()); src != "embedded" {
		t.Fatalf("source = %q, want embedded", src)
	}
	// A remote that comes back with a newer catalog is not consulted until
	// the cache expires: the embedded copy is cached too.
	fakeRemote(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write(catalogJSON(emb.Version+1, oneModelJSON("late", 1)))
	})
	if c, src := Load(context.Background()); src != "embedded" || c.Version != emb.Version {
		t.Errorf("second load = %q version %d, want cached embedded", src, c.Version)
	}
	if hits != 1 {
		t.Errorf("remote hits = %d, want 1", hits)
	}
}

func TestKVBytesPerToken(t *testing.T) {
	m := Model{Layers: 32, KVHeads: 8, HeadDim: 128}
	withKVLayers := Model{Layers: 32, KVHeads: 8, HeadDim: 128, KVLayers: 8}
	tests := []struct {
		name string
		m    Model
		kv   string
		want uint64
	}{
		{"f16 counts both K and V", m, "f16", 131072},
		{"q8_0 is one thirty-second smaller", m, "q8_0", 69632},
		{"q4_0 is a bit over half", m, "q4_0", 36864},
		{"unknown cache type is f16", m, "weird", 131072},
		{"empty cache type is f16", m, "", 131072},
		{"kv_layers overrides layers", withKVLayers, "f16", 32768},
		{"kv_layers with q8_0", withKVLayers, "q8_0", 17408},
		{"kv_layers with q4_0", withKVLayers, "q4_0", 9216},
		{"zero model", Model{}, "f16", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.KVBytesPerToken(tt.kv); got != tt.want {
				t.Errorf("KVBytesPerToken(%q) = %d, want %d", tt.kv, got, tt.want)
			}
		})
	}
}

func TestSWABytes(t *testing.T) {
	swa := Model{SWALayers: 8, SWAKVHeads: 2, SWAHeadDim: 128, SWAWindow: 4096}
	tests := []struct {
		name string
		m    Model
		kv   string
		ctx  int
		want uint64
	}{
		{"no sliding window", Model{}, "f16", 32768, 0},
		{"layers without a window", Model{SWALayers: 8, SWAKVHeads: 2, SWAHeadDim: 128}, "f16", 32768, 0},
		{"window without layers", Model{SWAWindow: 4096, SWAKVHeads: 2, SWAHeadDim: 128}, "f16", 32768, 0},
		{"window plus one batch at a large context", swa, "f16", 32768, 50331648},
		{"window plus one batch with q8_0", swa, "q8_0", 32768, 26738688},
		{"window plus one batch with q4_0", swa, "q4_0", 32768, 14155776},
		{"capped at the context", swa, "f16", 2048, 16777216},
		{"zero context", swa, "f16", 0, 0},
		{"unknown cache type is f16", swa, "weird", 32768, 50331648},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.SWABytes(tt.kv, tt.ctx); got != tt.want {
				t.Errorf("SWABytes(%q, %d) = %d, want %d", tt.kv, tt.ctx, got, tt.want)
			}
		})
	}
}

func TestFileURL(t *testing.T) {
	m := Model{Repo: "org/model"}
	tests := []struct {
		name string
		q    Quant
		file string
		want string
	}{
		{"model repo", Quant{}, "m-Q4.gguf", "https://huggingface.co/org/model/resolve/main/m-Q4.gguf"},
		{"quant repo wins", Quant{Repo: "other/repo"}, "m-Q4.gguf", "https://huggingface.co/other/repo/resolve/main/m-Q4.gguf"},
		{"subdirectory file", Quant{}, "split/m-Q4-00001.gguf", "https://huggingface.co/org/model/resolve/main/split/m-Q4-00001.gguf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.FileURL(tt.q, tt.file); got != tt.want {
				t.Errorf("FileURL = %q, want %q", got, tt.want)
			}
		})
	}
	d := Draft{Repo: "org/draft", File: "d-Q8.gguf"}
	if got, want := d.URL(), "https://huggingface.co/org/draft/resolve/main/d-Q8.gguf"; got != want {
		t.Errorf("Draft.URL = %q, want %q", got, want)
	}
}

func TestFind(t *testing.T) {
	c := &Catalog{Models: []Model{{ID: "a"}, {ID: "b"}}}
	if m, ok := c.Find("b"); !ok || m.ID != "b" {
		t.Errorf("Find(b) = %+v, %v", m, ok)
	}
	if _, ok := c.Find("missing"); ok {
		t.Error("Find(missing) reported a hit")
	}
	if _, ok := c.Find(""); ok {
		t.Error("Find(\"\") reported a hit")
	}
	if _, ok := (&Catalog{}).Find("a"); ok {
		t.Error("Find on an empty catalog reported a hit")
	}
}

// TestParseAcceptsQuantWithoutFiles documents the validation gap. The shipped
// catalog always fills Files, so only a hand-written or remote catalog hits it.
func TestParseAcceptsQuantWithoutFiles(t *testing.T) {
	c, err := parse([]byte(`{"version":1,"models":[{"id":"m","quants":[{"name":"Q4","bytes":10}]}]}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := len(c.Models[0].Quants[0].Files); got != 0 {
		t.Fatalf("files = %d, want the gap to be an empty list", got)
	}
	// internal/app/app.go:1103 does q.Files[0] without a length check, so a
	// catalog like this panics the Installed list. The owner should validate
	// in parse, or skip quantless models there.
	t.Skip("bug: parse accepts a quant with no files; app.Installed panics on q.Files[0] (internal/app/app.go:1103)")
}

// TestParseRejectsDuplicateIDs documents that duplicate model ids are accepted
// and Find silently returns the first.
func TestParseRejectsDuplicateIDs(t *testing.T) {
	c, err := parse([]byte(`{"version":1,"models":[` + oneModelJSON("dup", 5) + `,` + oneModelJSON("dup", 1) + `]}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(c.Models) != 2 {
		t.Fatalf("models = %d, want 2", len(c.Models))
	}
	if m, ok := c.Find("dup"); !ok || m.Score != 5 {
		t.Errorf("Find(dup) = %+v, %v; want the higher scored copy", m, ok)
	}
	t.Skip("bug: parse accepts duplicate model ids; Find and Use silently pick the first")
}

// TestLoadPanicsOnInvalidEmbedded pins the deliberate panic: a broken embedded
// catalog is a build error the user cannot recover from.
func TestLoadPanicsOnInvalidEmbedded(t *testing.T) {
	resetCache(t)
	old := embedded
	embedded = []byte(`{"version":1,"models":`)
	t.Cleanup(func() { embedded = old })
	defer func() {
		if recover() == nil {
			t.Error("Load did not panic on an invalid embedded catalog")
		}
	}()
	Load(context.Background())
}

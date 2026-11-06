package catalog

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The embedded catalog is the offline fallback every install ships, and the
// memory planner sizes models from its numbers. A wrong size or split count
// means a failed download; a wrong layer or context count means a crash or a
// frozen machine. These checks are structural and hermetic: they read the
// embedded copy and prove it agrees with itself. They do not talk to Hugging
// Face, so they cannot prove a file still exists at the URL or has that size;
// scripts/verify_catalog.py does that with GGUF header range requests.

// splitName matches the suffix llama.cpp uses to find the parts of a split
// model: <name>-00001-of-00003.gguf.
var splitName = regexp.MustCompile(`-(\d{5})-of-(\d{5})\.gguf$`)

// TestEmbeddedCatalogConsistency loads the embedded catalog and checks that it
// is internally consistent.
func TestEmbeddedCatalogConsistency(t *testing.T) {
	c, err := parse(embedded)
	if err != nil {
		t.Fatalf("embedded catalog: %v", err)
	}
	if len(c.Models) == 0 {
		t.Fatal("embedded catalog has no models")
	}

	t.Run("ModelIDsUnique", func(t *testing.T) {
		seen := make(map[string]bool, len(c.Models))
		for _, m := range c.Models {
			if m.ID == "" {
				t.Errorf("model %q has an empty id", m.Name)
				continue
			}
			if seen[m.ID] {
				t.Errorf("model id %q is listed twice", m.ID)
			}
			seen[m.ID] = true
		}
	})

	t.Run("RanksUniqueAndContiguous", func(t *testing.T) {
		// The catalog is a ranked list: Load sorts models smartest first, so a
		// model's rank is its 1-based position in that order. Ranks must be
		// unique (one model per position) and contiguous (1..len, no gaps).
		// Scores may tie; sortModels is stable, so the file order breaks the tie.
		seen := make(map[int]bool, len(c.Models))
		for i, m := range c.Models {
			rank := i + 1
			if seen[rank] {
				t.Errorf("rank %d is held by more than one model, last %q", rank, m.ID)
			}
			seen[rank] = true
			if i > 0 && c.Models[i-1].Score < m.Score {
				t.Errorf("%q (score %v, rank %d) is ranked above %q (score %v, rank %d)",
					m.ID, m.Score, rank, c.Models[i-1].ID, c.Models[i-1].Score, i)
			}
		}
		for rank := 1; rank <= len(c.Models); rank++ {
			if !seen[rank] {
				t.Errorf("rank %d is missing", rank)
			}
		}
	})

	t.Run("QuantsSortedBySize", func(t *testing.T) {
		// Quants are listed cheapest first, one group per repo, so a caller can
		// walk the list and stop at the first quant that fits. Sizes must be
		// strictly increasing inside a group: an equal size would make "the
		// next smaller quant" ambiguous.
		type entry struct {
			name  string
			bytes uint64
		}
		type key struct {
			model string
			repo  string
		}
		last := make(map[key]entry)
		for _, m := range c.Models {
			if len(m.Quants) == 0 {
				t.Errorf("%s: no quants", m.ID)
			}
			for _, q := range m.Quants {
				if q.Bytes == 0 {
					t.Errorf("%s/%s: bytes is 0", m.ID, q.Name)
				}
				if len(q.Files) == 0 {
					t.Errorf("%s/%s: no files", m.ID, q.Name)
				}
				repo := q.Repo
				if repo == "" {
					repo = m.Repo
				}
				k := key{m.ID, repo}
				if prev, ok := last[k]; ok && q.Bytes <= prev.bytes {
					t.Errorf("%s: quant %q (%d bytes) is listed after %q (%d bytes) in %s",
						m.ID, q.Name, q.Bytes, prev.name, prev.bytes, repo)
				}
				last[k] = entry{q.Name, q.Bytes}
			}
		}
	})

	t.Run("SplitFilesComplete", func(t *testing.T) {
		// llama.cpp finds the parts of a split model by name, and EZLM saves
		// each part under its own name: the numbering has to match the list.
		for _, m := range c.Models {
			for _, q := range m.Quants {
				if len(q.Files) == 1 {
					if splitName.MatchString(q.Files[0]) {
						t.Errorf("%s/%s: single file %q carries a split suffix", m.ID, q.Name, q.Files[0])
					}
					continue
				}
				prefix := ""
				for i, f := range q.Files {
					mm := splitName.FindStringSubmatch(f)
					if mm == nil {
						t.Errorf("%s/%s: part %d %q is not named -NNNNN-of-NNNNN.gguf", m.ID, q.Name, i+1, f)
						continue
					}
					part, _ := strconv.Atoi(mm[1])
					total, _ := strconv.Atoi(mm[2])
					if part != i+1 {
						t.Errorf("%s/%s: part %d is numbered %05d", m.ID, q.Name, i+1, part)
					}
					if total != len(q.Files) {
						t.Errorf("%s/%s: lists %d parts but %q says %d", m.ID, q.Name, len(q.Files), f, total)
					}
					base := strings.TrimSuffix(f, mm[0])
					if i == 0 {
						prefix = base
					} else if base != prefix {
						t.Errorf("%s/%s: part %d is %q, part 1 is %q", m.ID, q.Name, i+1, base, prefix)
					}
				}
			}
		}
	})

	t.Run("URLs", func(t *testing.T) {
		for _, m := range c.Models {
			if m.Repo == "" {
				t.Errorf("%s: no repo", m.ID)
			} else {
				checkRepo(t, m.ID, m.Repo)
			}
			for _, q := range m.Quants {
				if q.Repo != "" {
					checkRepo(t, m.ID+"/"+q.Name, q.Repo)
				}
				for _, f := range q.Files {
					if strings.HasPrefix(f, "/") {
						t.Errorf("%s/%s: file %q starts with a slash", m.ID, q.Name, f)
					}
					checkURL(t, m.ID+"/"+q.Name, m.FileURL(q, f))
				}
			}
			if m.Draft != nil {
				checkURL(t, m.ID+" draft", m.Draft.URL())
			}
		}
	})

	t.Run("PlannerFields", func(t *testing.T) {
		// These feed the memory plan: a zero layer or context count sizes the
		// KV cache at nothing.
		for _, m := range c.Models {
			if m.CtxMax <= 0 {
				t.Errorf("%s: ctx_max is %d", m.ID, m.CtxMax)
			}
			if m.Layers <= 0 {
				t.Errorf("%s: layers is %d", m.ID, m.Layers)
			}
			if m.KVHeads <= 0 {
				t.Errorf("%s: kv_heads is %d", m.ID, m.KVHeads)
			}
			if m.HeadDim <= 0 {
				t.Errorf("%s: head_dim is %d", m.ID, m.HeadDim)
			}
		}
	})
}

// checkRepo checks that a repo is an owner/name pair, not a URL.
func checkRepo(t *testing.T, what, repo string) {
	t.Helper()
	if !strings.Contains(repo, "/") {
		t.Errorf("%s: repo %q is not owner/name", what, repo)
	}
	if strings.Contains(repo, "://") {
		t.Errorf("%s: repo %q is a URL, not owner/name", what, repo)
	}
}

// checkURL checks that a download URL is https and on huggingface.co.
func checkURL(t *testing.T, what, raw string) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Errorf("%s: %q: %v", what, raw, err)
		return
	}
	if u.Scheme != "https" {
		t.Errorf("%s: %q is not https", what, raw)
	}
	if u.Host != "huggingface.co" {
		t.Errorf("%s: %q is not on huggingface.co", what, raw)
	}
}

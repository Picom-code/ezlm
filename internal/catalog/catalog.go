// Package catalog holds the ranked list of models EZLM knows how to deploy.
//
// The catalog is fetched from RemoteURL so rankings can be updated without a
// new release; the embedded copy is the offline fallback.
package catalog

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

//go:embed catalog.json
var embedded []byte

// RemoteURL is where updated catalogs are published.
var RemoteURL = "https://raw.githubusercontent.com/picom-code/ezlm/main/internal/catalog/catalog.json"

type Quant struct {
	Name  string   `json:"name"`
	Repo  string   `json:"repo,omitempty"` // when the file lives outside the model's main repo
	Files []string `json:"files"`
	Bytes uint64   `json:"bytes"`
	// MTP is true when the file embeds multi-token-prediction heads, enabling
	// self-speculative decoding (--spec-type draft-mtp).
	MTP bool `json:"mtp,omitempty"`
	// Quality overrides the default quality factor for this quant name (0-1),
	// for community quants that measure better than their bit width suggests.
	Quality float64 `json:"quality,omitempty"`
	// LazyBytes can stay on disk with --lazy-mode on (e.g. n-gram tables).
	LazyBytes uint64 `json:"lazy_bytes,omitempty"`
	// Args are recipe flags known to help this file (from community testing).
	Args []string `json:"args,omitempty"`
}

// Draft is a separate speculative-decoding model file.
type Draft struct {
	Repo  string `json:"repo"`
	File  string `json:"file"`
	Bytes uint64 `json:"bytes"`
	Type  string `json:"type"` // draft-mtp | draft-dflash | draft-dspark
}

type Model struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Family        string  `json:"family"`
	License       string  `json:"license"`
	ParamsB       float64 `json:"params_b"`
	ActiveParamsB float64 `json:"active_params_b"`
	MoE           bool    `json:"moe"`
	Score         float64 `json:"score"`
	ScoreSource   string  `json:"score_source"`
	Layers        int     `json:"layers"`
	KVHeads       int     `json:"kv_heads"`
	HeadDim       int     `json:"head_dim"`
	KVLayers      int     `json:"kv_layers"`
	CtxMax        int     `json:"ctx_max"`
	ToolCalling   bool    `json:"tool_calling"`
	Repo          string  `json:"repo"`
	Quants        []Quant `json:"quants"`

	Vendor    string `json:"vendor,omitempty"`
	Country   string `json:"country,omitempty"` // ISO 3166 alpha-2
	Vision    bool   `json:"vision,omitempty"`
	NoKVQuant bool   `json:"no_kv_quant,omitempty"` // quantized KV cache hurts this model (e.g. gpt-oss)
	Draft     *Draft `json:"draft,omitempty"`
	// Native1Bit marks models trained at ~1-bit (e.g. Bonsai): their tiny
	// quants are the real thing, not a degraded copy.
	Native1Bit bool   `json:"native_1bit,omitempty"`
	Notes      string `json:"notes,omitempty"`

	// Sliding-window attention: layers that keep only the last SWAWindow
	// tokens. Their cache doesn't grow with the context, but it isn't free
	// (Gemma 4 31B: ~1-2 GB), so plans must count it.
	SWALayers  int `json:"swa_layers,omitempty"`
	SWAKVHeads int `json:"swa_kv_heads,omitempty"`
	SWAHeadDim int `json:"swa_head_dim,omitempty"`
	SWAWindow  int `json:"swa_window,omitempty"`
}

// KVBytesPerToken is the K+V cache cost of one token of context at the given
// cache type (f16, q8_0, q4_0).
func (m Model) KVBytesPerToken(kv string) uint64 {
	l := m.KVLayers
	if l == 0 {
		l = m.Layers
	}
	elems := uint64(2 * l * m.KVHeads * m.HeadDim)
	switch kv {
	case "q8_0":
		return elems * 34 / 32 // 1.0625 bytes per element
	case "q4_0":
		return elems * 18 / 32 // 0.5625 bytes per element
	}
	return elems * 2
}

// SWABytes is the sliding-window cache at the given context and cache type.
// llama.cpp sizes it to the window plus one batch (EZLM uses batches of up
// to 2048 tokens), capped at the context.
func (m Model) SWABytes(kv string, ctx int) uint64 {
	if m.SWALayers == 0 || m.SWAWindow == 0 {
		return 0
	}
	tokens := uint64(min(ctx, m.SWAWindow+2048))
	elems := uint64(2 * m.SWALayers * m.SWAKVHeads * m.SWAHeadDim)
	switch kv {
	case "q8_0":
		return elems * 34 / 32 * tokens
	case "q4_0":
		return elems * 18 / 32 * tokens
	}
	return elems * 2 * tokens
}

// FileURL is the HuggingFace download URL for a file of a quant.
func (m Model) FileURL(q Quant, file string) string {
	repo := m.Repo
	if q.Repo != "" {
		repo = q.Repo
	}
	return fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repo, file)
}

// DraftURL is the download URL of the model's separate draft file.
func (d Draft) URL() string {
	return fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", d.Repo, d.File)
}

type Catalog struct {
	Version int     `json:"version"`
	Models  []Model `json:"models"`
	// Tools pins the app versions EZLM installs (OpenCode, Node, DeepSeek
	// Harness), in internal/integrate's Pin format. Absent: the pins built
	// into this version apply.
	Tools json.RawMessage `json:"tools,omitempty"`
}

// Load returns the remote catalog if reachable and valid, else the embedded one.
// EZLM_CATALOG may point at a local file (used in development).
func Load(ctx context.Context) (*Catalog, string) {
	if p := os.Getenv("EZLM_CATALOG"); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			if c, err := parse(b); err == nil {
				return c, "file:" + p
			}
		}
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if cached != nil && time.Since(cachedAt) < 30*time.Minute {
		return cached, cachedSrc
	}
	emb, err := parse(embedded)
	if err != nil {
		panic("embedded catalog is invalid: " + err.Error())
	}
	cached, cachedSrc, cachedAt = emb, "embedded", time.Now()
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if req, err := http.NewRequestWithContext(ctx, "GET", RemoteURL, nil); err == nil {
		if resp, err := http.DefaultClient.Do(req); err == nil {
			defer resp.Body.Close()
			var c Catalog
			// Only a newer catalog replaces the one built into this version,
			// never an older published copy.
			if resp.StatusCode == 200 && json.NewDecoder(resp.Body).Decode(&c) == nil && len(c.Models) > 0 && c.Version > emb.Version {
				sortModels(&c)
				cached, cachedSrc = &c, "remote"
			}
		}
	}
	return cached, cachedSrc
}

var (
	cacheMu   sync.Mutex
	cached    *Catalog
	cachedSrc string
	cachedAt  time.Time
)

func parse(b []byte) (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if len(c.Models) == 0 {
		return nil, fmt.Errorf("catalog has no models")
	}
	sortModels(&c)
	return &c, nil
}

// sortModels orders models smartest first.
func sortModels(c *Catalog) {
	sort.SliceStable(c.Models, func(i, j int) bool { return c.Models[i].Score > c.Models[j].Score })
}

// Find returns the model with the given id.
func (c *Catalog) Find(id string) (Model, bool) {
	for _, m := range c.Models {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

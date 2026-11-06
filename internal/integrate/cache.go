package integrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/picom-code/ezlm/internal/paths"
)

// The integrations cache, ~/.ezlm/integrations.json: probe results (so
// Targets never has to run a program), each target's last check, and every
// port EZLM's server has used (so a config pointing at an old port still
// counts as EZLM's).

// Probe is what running a program told EZLM about it. It stays valid while
// the file's size and modification time are unchanged.
type Probe struct {
	Version string            `json:"version,omitempty"`
	Arch    string            `json:"arch,omitempty"`
	Facts   map[string]string `json:"facts,omitempty"` // tool-specific, e.g. opencode "data": "v2"
	Size    int64             `json:"size"`
	ModTime time.Time         `json:"mtime"`
	At      time.Time         `json:"at"`
}

type cacheFile struct {
	Probes map[string]Probe       `json:"probes,omitempty"` // by absolute program path
	Checks map[string]CheckResult `json:"checks,omitempty"` // by tool id
	Before map[string]CheckResult `json:"before,omitempty"` // by tool id: the result a failed launch replaced
	Ports  []int                  `json:"ports,omitempty"`
}

var cacheMu sync.Mutex

func cachePath() string { return filepath.Join(paths.Home(), "integrations.json") }

func loadCache() cacheFile {
	var c cacheFile
	if b, err := os.ReadFile(cachePath()); err == nil {
		json.Unmarshal(b, &c)
	}
	return c
}

func editCache(fn func(*cacheFile)) error {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	c := loadCache()
	fn(&c)
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(cachePath(), append(b, '\n'), 0o600)
}

// CachedProbe returns the saved probe of bin if the file hasn't changed.
func CachedProbe(bin string) (Probe, bool) {
	st, err := os.Stat(bin)
	if err != nil {
		return Probe{}, false
	}
	cacheMu.Lock()
	p, ok := loadCache().Probes[bin]
	cacheMu.Unlock()
	if !ok || p.Size != st.Size() || !p.ModTime.Equal(st.ModTime()) {
		return Probe{}, false
	}
	return p, true
}

// SaveProbe records a probe of bin, stamped with its size and mtime.
func SaveProbe(bin string, p Probe) error {
	st, err := os.Stat(bin)
	if err != nil {
		return err
	}
	p.Size, p.ModTime, p.At = st.Size(), st.ModTime(), time.Now()
	return editCache(func(c *cacheFile) {
		if c.Probes == nil {
			c.Probes = map[string]Probe{}
		}
		c.Probes[bin] = p
	})
}

// LastCheck is the newest check or launch result of a target (nil if none).
func LastCheck(id string) *CheckResult {
	cacheMu.Lock()
	r, ok := loadCache().Checks[id]
	cacheMu.Unlock()
	if !ok {
		return nil
	}
	return &r
}

// SaveCheck records a target's newest check or launch result. A failed
// launch keeps the result it replaces, so ClearLaunchFailure can bring the
// last check back once the app opens fine.
func SaveCheck(id string, r CheckResult) error {
	return editCache(func(c *cacheFile) {
		if c.Checks == nil {
			c.Checks = map[string]CheckResult{}
		}
		prev, had := c.Checks[id]
		switch {
		case !r.OK && r.Stage == StageLaunch && had && !(prev.Stage == StageLaunch && !prev.OK):
			if c.Before == nil {
				c.Before = map[string]CheckResult{}
			}
			c.Before[id] = prev
		case r.Stage != StageLaunch:
			delete(c.Before, id)
		}
		c.Checks[id] = r
	})
}

// ClearLaunchFailure: the app opened fine after a failed launch, so its tile
// stops saying "Couldn't open last time". The check result from before the
// failure comes back; with none, the failure is simply dropped.
func ClearLaunchFailure(id string) error {
	if r := LastCheck(id); r == nil || r.OK || r.Stage != StageLaunch {
		return nil
	}
	return editCache(func(c *cacheFile) {
		if prev, ok := c.Before[id]; ok {
			c.Checks[id] = prev
			delete(c.Before, id)
		} else {
			delete(c.Checks, id)
		}
	})
}

// NotePort remembers a port EZLM's server has used.
func NotePort(port int) error {
	if slices.Contains(KnownPorts(), port) {
		return nil
	}
	return editCache(func(c *cacheFile) {
		if !slices.Contains(c.Ports, port) {
			c.Ports = append(c.Ports, port)
		}
	})
}

// KnownPorts lists every port EZLM's server has used.
func KnownPorts() []int {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	return loadCache().Ports
}

package integrate

import (
	"sort"
	"sync"
)

var (
	regMu sync.RWMutex
	reg   = map[string]Tool{}
)

// Register adds a tool. Each tool calls it from its own file's init().
func Register(t Tool) {
	regMu.Lock()
	defer regMu.Unlock()
	id := t.Info().ID
	if _, dup := reg[id]; dup {
		panic("integrate: tool registered twice: " + id)
	}
	reg[id] = t
}

// Replace swaps in a tool (tests use it for a fake) and returns a function
// that puts the previous one back.
func Replace(t Tool) (restore func()) {
	regMu.Lock()
	defer regMu.Unlock()
	id := t.Info().ID
	prev, had := reg[id]
	reg[id] = t
	return func() {
		regMu.Lock()
		defer regMu.Unlock()
		if had {
			reg[id] = prev
		} else {
			delete(reg, id)
		}
	}
}

// Lookup finds a registered tool by id.
func Lookup(id string) (Tool, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	t, ok := reg[id]
	return t, ok
}

// All lists the registered tools in tile order.
func All() []Tool {
	regMu.RLock()
	out := make([]Tool, 0, len(reg))
	for _, t := range reg {
		out = append(out, t)
	}
	regMu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Info(), out[j].Info()
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return a.ID < b.ID
	})
	return out
}

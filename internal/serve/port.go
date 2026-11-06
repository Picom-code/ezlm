package serve

import (
	"fmt"
	"strings"
)

// BindFailed reports whether a start failed because another program took the
// port between the free check and the server binding it. The check and the
// bind are not one step: a dev tool, a proxy, or another model server can
// take the port in the gap (a split model is pre-read first, which can take
// minutes), and llama-server then exits with one of these lines.
func BindFailed(log string) bool {
	l := strings.ToLower(log)
	return strings.Contains(l, "address already in use") || strings.Contains(l, "couldn't bind")
}

// MoveToFreePort moves c off the port it has, which another program took
// between the free check and the bind, to the first free port after it, and
// saves the config. It returns the port it left and the port it moved to.
// Callers must point connected apps at the new port.
//
// It never saves a port known to be taken: when nothing is free it leaves the
// config alone and says so.
func MoveToFreePort(c *Config) (old, port int, err error) {
	old = c.Port
	port = freePortFrom(old + 1)
	if port == 0 {
		return old, old, fmt.Errorf("no free port after %d", old)
	}
	c.Port = port
	if err := c.Save(); err != nil {
		c.Port = old
		return old, old, err
	}
	return old, port, nil
}

// freePortFrom is FreePort, but it returns 0 rather than a port known to be
// busy when the whole range is taken.
func freePortFrom(start int) int {
	if start < 1 {
		start = 1
	}
	for p := start; p <= 65535; p++ {
		if PortFree(p) {
			return p
		}
	}
	return 0
}

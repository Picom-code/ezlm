// Package awake keeps the computer from falling asleep on its own while EZLM
// works, so a long download doesn't stall when the person walks away. It only
// stops idle sleep: closing the lid or choosing Sleep still works, and the
// screen may still turn off.
package awake

import "sync"

// Hold keeps the computer awake until the returned release is called or EZLM
// exits, whichever comes first. It does nothing where that isn't possible.
func Hold(why string) (release func()) {
	stop := hold(why)
	var once sync.Once
	return func() { once.Do(stop) }
}

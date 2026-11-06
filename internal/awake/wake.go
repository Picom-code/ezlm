package awake

import (
	"context"
	"time"
)

// Watcher notices that the computer has been asleep. Go's monotonic clock
// stops while the machine sleeps and the wall clock keeps going, so a wake
// shows up as a jump in wall time that the monotonic clock never saw. A long
// sleep can leave the model server dead or stalled, so after a wake the caller
// checks it again.
type Watcher struct {
	// Every is how often to look. Zero means once a minute.
	Every time.Duration
	// Slack is how much longer than Every a gap may be before it counts as a
	// wake; a busy machine can overshoot a tick. Zero means 15 seconds.
	Slack time.Duration
	// Now returns the wall clock and the monotonic clock. Tests replace it.
	// The default is time.Now, which carries both.
	Now func() (wall, mono time.Time)
}

// Run calls wake with the time the machine was probably asleep, once per wake,
// until ctx is done. wake runs on Run's goroutine, so keep it quick.
func (w *Watcher) Run(ctx context.Context, wake func(slept time.Duration)) {
	every := w.Every
	if every <= 0 {
		every = time.Minute
	}
	slack := w.Slack
	if slack <= 0 {
		slack = 15 * time.Second
	}
	now := w.Now
	if now == nil {
		now = func() (time.Time, time.Time) {
			t := time.Now()
			return t.Round(0), t // Round(0) drops the monotonic reading.
		}
	}
	prevWall, prevMono := now()
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		wall, mono := now()
		wallGap, monoGap := wall.Sub(prevWall), mono.Sub(prevMono)
		prevWall, prevMono = wall, mono
		if monoGap < 0 {
			continue // the clocks disagree; wait for the next tick
		}
		if slept := wallGap - monoGap; slept > slack {
			wake(slept)
		}
	}
}

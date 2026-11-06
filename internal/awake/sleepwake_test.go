package awake

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeClock drives a Watcher from a script instead of the real clock.
type fakeClock struct {
	mu   sync.Mutex
	wall time.Time
	mono time.Time
}

func newFakeClock() *fakeClock {
	t := time.Unix(1700000000, 0)
	return &fakeClock{wall: t, mono: t}
}

func (c *fakeClock) Now() (time.Time, time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.wall, c.mono
}

// advance moves the wall clock and the monotonic clock by different amounts.
func (c *fakeClock) advance(wall, mono time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wall = c.wall.Add(wall)
	c.mono = c.mono.Add(mono)
}

// runWatcher starts a Watcher and returns its wake reports and a stop function
// that waits for Run to return.
func runWatcher(t *testing.T, w *Watcher) (<-chan time.Duration, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	wakes := make(chan time.Duration, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx, func(slept time.Duration) { wakes <- slept })
	}()
	stop := func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not return after cancel")
		}
	}
	return wakes, stop
}

// A machine that is merely busy ticks a little late; that is not a wake.
func TestWatcherIgnoresNormalTicks(t *testing.T) {
	c := newFakeClock()
	wakes, stop := runWatcher(t, &Watcher{Every: 2 * time.Millisecond, Slack: time.Second, Now: c.Now})
	defer stop()
	for i := 0; i < 10; i++ {
		c.advance(3*time.Millisecond, 2*time.Millisecond)
		time.Sleep(3 * time.Millisecond)
	}
	select {
	case slept := <-wakes:
		t.Fatalf("false wake of %v", slept)
	default:
	}
}

// A gap the monotonic clock did not see is a sleep, and the report is roughly
// how long it lasted.
func TestWatcherReportsSleep(t *testing.T) {
	c := newFakeClock()
	wakes, stop := runWatcher(t, &Watcher{Every: 2 * time.Millisecond, Slack: 100 * time.Millisecond, Now: c.Now})
	defer stop()
	time.Sleep(10 * time.Millisecond) // let a few normal ticks pass
	c.advance(2*time.Hour, 2*time.Millisecond)
	select {
	case slept := <-wakes:
		if slept < time.Hour || slept > 3*time.Hour {
			t.Fatalf("reported a %v sleep, want about 2h", slept)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no wake reported after a two hour sleep")
	}
}

// A wall clock stepped backwards by NTP is not a sleep and must not be
// reported as one.
func TestWatcherIgnoresBackwardsWallStep(t *testing.T) {
	c := newFakeClock()
	wakes, stop := runWatcher(t, &Watcher{Every: 2 * time.Millisecond, Slack: 100 * time.Millisecond, Now: c.Now})
	defer stop()
	c.advance(-time.Hour, 2*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	select {
	case slept := <-wakes:
		t.Fatalf("backwards wall step reported as a %v wake", slept)
	default:
	}
}

// A gap under the slack is a slow machine, not a sleep.
func TestWatcherHonoursSlack(t *testing.T) {
	c := newFakeClock()
	wakes, stop := runWatcher(t, &Watcher{Every: 2 * time.Millisecond, Slack: 50 * time.Millisecond, Now: c.Now})
	defer stop()
	c.advance(20*time.Millisecond, 2*time.Millisecond) // 18ms of extra wall time
	time.Sleep(30 * time.Millisecond)
	select {
	case slept := <-wakes:
		t.Fatalf("18ms gap under a 50ms slack reported as a %v wake", slept)
	default:
	}
}

// With the real clock and default settings a watcher that is not asleep stays
// quiet, and the defaults themselves are usable.
func TestWatcherDefaults(t *testing.T) {
	wakes, stop := runWatcher(t, &Watcher{Every: 2 * time.Millisecond})
	defer stop()
	time.Sleep(40 * time.Millisecond)
	select {
	case slept := <-wakes:
		t.Fatalf("real clock reported a %v wake", slept)
	default:
	}
}

// An already cancelled context returns at once and never reports a wake.
func TestWatcherStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newFakeClock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		(&Watcher{Every: time.Millisecond, Now: c.Now}).Run(ctx, func(time.Duration) {
			t.Error("wake reported after cancel")
		})
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return for a cancelled context")
	}
}

// A released hold can be released again from several goroutines at once, and
// none of them blocks.
func TestHoldReleaseIsConcurrentSafe(t *testing.T) {
	release := Hold("test")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); release() }()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent release hung")
	}
}

// An empty reason must not panic or stop the hold from being released.
func TestHoldEmptyReason(t *testing.T) {
	release := Hold("")
	release()
	release()
}

package keys

import (
	"crypto/subtle"
	"strings"
	"testing"
	"time"
)

func TestEqual(t *testing.T) {
	const token = "ezlm-0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcd"
	for _, tc := range []struct {
		name string
		a, b string
		want bool
	}{
		{"same", token, token, true},
		{"last byte off", token, token[:len(token)-1] + "e", false},
		{"first byte off", token, "f" + token[1:], false},
		{"prefix of the token", token[:len(token)-1], token, false},
		{"both empty", "", "", false},
		{"expected empty", "", token, false},
		{"presented empty", token, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Equal(tc.a, tc.b); got != tc.want {
				t.Fatalf("Equal(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// Equal must reject two empty strings: subtle.ConstantTimeCompare returns 1
// for them, and a server started with an empty token would then accept a
// request that presents nothing. TestEqual covers the behaviour; this pins
// the reason for the guard so nobody "simplifies" it away.
func TestSubtleEmptyGotcha(t *testing.T) {
	if subtle.ConstantTimeCompare(nil, nil) != 1 {
		t.Fatal("crypto/subtle changed: ConstantTimeCompare(nil, nil) is no longer 1, Equal's empty guard may be redundant")
	}
}

const timingKeySize = 1 << 18 // 256 KiB: long enough that an early exit is seconds of difference, not noise

func timingKey() string { return strings.Repeat("a", timingKeySize) }

// fastest returns the shortest per-call time over several rounds, which
// filters out scheduler noise better than an average.
func fastest(rounds, reps int, cmp func(a, b string) bool, a, b string) time.Duration {
	best := time.Duration(1 << 62)
	for r := 0; r < rounds; r++ {
		start := time.Now()
		for i := 0; i < reps; i++ {
			if cmp(a, b) {
				panic("timing comparison unexpectedly matched")
			}
		}
		if d := time.Since(start) / time.Duration(reps); d < best {
			best = d
		}
	}
	return best
}

func naiveEqual(a, b string) bool { return a == b }

// Equal scans the whole key, so a mismatch in the first byte costs the same
// as a mismatch in the last one. This is the test that fails if the
// implementation goes back to ==.
func TestEqualNoEarlyExit(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	key := timingKey()
	first := "b" + key[1:]
	last := key[:len(key)-1] + "b"

	firstNS := fastest(5, 20, Equal, first, key)
	lastNS := fastest(5, 20, Equal, last, key)
	t.Logf("Equal 256 KiB: first-byte mismatch %v, last-byte mismatch %v", firstNS, lastNS)
	if firstNS == 0 {
		firstNS = time.Nanosecond
	}
	if ratio := float64(lastNS) / float64(firstNS); ratio > 5 {
		t.Fatalf("Equal exits early: first-byte mismatch %v, last-byte mismatch %v (ratio %.1f)", firstNS, lastNS, ratio)
	}
}

// TestNaiveCompareHasTimingSignal proves the measurement above can see an
// early exit at all: with == the last-byte mismatch is far slower than the
// first-byte one. It is evidence for the report, not a test of Equal.
func TestNaiveCompareHasTimingSignal(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	key := timingKey()
	first := "b" + key[1:]
	last := key[:len(key)-1] + "b"

	firstNS := fastest(5, 20, naiveEqual, first, key)
	lastNS := fastest(5, 20, naiveEqual, last, key)
	t.Logf("== 256 KiB: first-byte mismatch %v, last-byte mismatch %v", firstNS, lastNS)
	if firstNS == 0 {
		firstNS = time.Nanosecond
	}
	if ratio := float64(lastNS) / float64(firstNS); ratio < 5 {
		t.Fatalf("== showed no timing signal (first %v, last %v, ratio %.1f); the no-early-exit test proves nothing", firstNS, lastNS, ratio)
	}
}

// The production token is 32 characters. Benchmarks below record how much
// the comparison costs there; run with -bench.
func BenchmarkEqual32(b *testing.B) {
	const token = "ezlm-0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcd"
	bad := token[:len(token)-1] + "e"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if Equal(bad, token) {
			b.Fatal("matched")
		}
	}
}

func BenchmarkNaiveEqual32(b *testing.B) {
	const token = "ezlm-0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcd"
	bad := token[:len(token)-1] + "e"
	for i := 0; i < b.N; i++ {
		if bad == token {
			b.Fatal("matched")
		}
	}
}

func BenchmarkEqual256K(b *testing.B) {
	key := timingKey()
	last := key[:len(key)-1] + "b"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if Equal(last, key) {
			b.Fatal("matched")
		}
	}
}

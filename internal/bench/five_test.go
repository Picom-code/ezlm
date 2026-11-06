package bench

import "testing"

func TestIsFive(t *testing.T) {
	for ans, want := range map[string]bool{
		"5": true, "Five": true, "five cents": true, "$0.05": true, "0.05": true, "5¢": true, "The ball costs 5 cents.": true,
		"10": false, "Ten": false, "$0.10": false, "0.1": false, "105": false, "15": false, "50": false, "": false,
		"5 (not 10)": false,
	} {
		if got := isFive(ans); got != want {
			t.Errorf("isFive(%q) = %v, want %v", ans, got, want)
		}
	}
}

package scan

import "testing"

// Lines as Windows gives them (from the WMI class; the counter fallback is
// the same shape with lowercase hex), including a virtual adapter's bogus
// terabytes, which must not count.
func TestParseGPUUsage(t *testing.T) {
	out := "pid_16640_luid_0x00000000_0x000110FB_phys_0 7687751671808\r\n" +
		"pid_2788_luid_0x00000000_0x00012874_phys_0 10273845248\r\n" +
		"pid_2044_luid_0x00000000_0x000110FB_phys_0 251236352\r\n" +
		"pid_2044_luid_0x00000000_0x00012874_phys_0 1048576\r\n" +
		"_Total 99\r\n\r\n"
	u := parseGPUUsage(out)
	if len(u) != 2 || u[2788] != 10273845248 || u[2044] != 251236352+1048576 {
		t.Fatalf("got %v", u)
	}
	if _, ok := u[16640]; ok {
		t.Fatal("a virtual adapter's impossible number was counted")
	}
	if len(parseGPUUsage("")) != 0 {
		t.Fatal("nothing in, nothing out")
	}
}

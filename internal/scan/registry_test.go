package scan

import (
	"encoding/binary"
	"testing"
)

// Drivers disagree on how they store a card's memory: a QWORD, 8 bytes of
// binary, or the older 4-byte form. All three have to count, or an Intel
// Arc with 8 or 16 GB reads as 4 GB (or as nothing) and models are picked
// for the wrong card.
func TestAdapterVRAMFromEveryDriverType(t *testing.T) {
	qword := func(v uint64) regValue {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, v)
		return regValue{bin: b}
	}
	dword := func(v uint32) regValue {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, v)
		return regValue{bin: b}
	}
	num := func(v uint64) regValue { return regValue{num: v, isNum: true} }

	cases := []struct {
		name, why string
		qw, mem   regValue
		want      uint64
	}{
		{"qw qword", "AMD and NVIDIA write a QWORD", num(8589934592), regValue{}, 8589934592},
		{"qw binary", "some drivers write the same 8 bytes as REG_BINARY", qword(17179869184), regValue{}, 17179869184},
		{"qw binary beats capped mem", "the 4-byte form caps at 4 GB", qword(17179869184), dword(0xFFFFFFFF), 17179869184},
		{"mem dword", "Intel writes MemorySize as a DWORD", regValue{}, num(4294967296), 4294967296},
		{"mem binary", "older drivers write 4 bytes of binary", regValue{}, dword(2147483648), 2147483648},
		{"qw binary too short", "3 bytes is no size at all", regValue{bin: []byte{1, 2, 3}}, dword(1073741824), 1073741824},
		{"none", "a card with neither value is unknown", regValue{}, regValue{}, 0},
	}
	for _, c := range cases {
		if got := adapterVRAM(c.qw, c.mem); got != c.want {
			t.Errorf("%s: adapterVRAM = %d, want %d (%s)", c.name, got, c.want, c.why)
		}
	}
}

// PowerShell prints one device ID per line, but on a localized Windows a
// message can share the output. Words of a message are not device IDs.
func TestDeviceIDsSkipsMessagesAndBlankLines(t *testing.T) {
	out := "PCI\\VEN_1002&DEV_73BF&SUBSYS_00000000&REV_C0\\6&1A2B3C4D&0&0008\r\n" +
		"\r\n" +
		"   \r\n" +
		"ROOT\\BasicDisplay\\0000\r\n" +
		"Get-CimInstance : Der Computer kann nicht gefunden werden.\r\n"
	got := deviceIDs(out)
	want := []string{
		`PCI\VEN_1002&DEV_73BF&SUBSYS_00000000&REV_C0\6&1A2B3C4D&0&0008`,
		`ROOT\BasicDisplay\0000`,
	}
	if len(got) != len(want) {
		t.Fatalf("deviceIDs = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("deviceIDs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

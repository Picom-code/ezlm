//go:build windows

package scan

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// A card's memory comes from whichever value its driver writes. An empty
// value must not hide the card, and an 8-byte binary value must not be read
// as its low half (8 GB would come out as 0).
func TestAdapterVRAM(t *testing.T) {
	ptr := func(v uint64) *uint64 { return &v }
	qword := func(v uint64) []byte {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, v)
		return b
	}
	dword := func(v uint32) []byte {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, v)
		return b
	}
	for _, c := range []struct {
		name string
		qw   *uint64
		bin  []byte
		dw   *uint64
		want uint64
	}{
		{"current driver", ptr(16 * GB), nil, nil, 16 * GB},
		{"empty 64-bit value falls through", ptr(0), dword(512 << 20), nil, 512 << 20},
		{"4-byte binary value", nil, dword(1 << 30), nil, 1 << 30},
		{"8-byte binary value", nil, qword(8 * GB), nil, 8 * GB},
		{"8-byte binary beats a zero 64-bit value", ptr(0), qword(8 * GB), nil, 8 * GB},
		{"DWORD value", nil, nil, ptr(2 * GB), 2 * GB},
		{"nothing at all", nil, nil, nil, 0},
		{"too short to read", nil, []byte{1, 2}, ptr(4 * GB), 4 * GB},
		{"all empty", ptr(0), dword(0), ptr(0), 0},
	} {
		if got := adapterVRAM(c.qw, c.bin, c.dw); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

// The adapter list is whatever PowerShell prints: one device path per line,
// possibly with a warning line or a byte order mark in front of it.
func TestDeviceIDs(t *testing.T) {
	out := "\ufeffPCI\\VEN_1002&DEV_73BF&SUBSYS_1234&REV_C1\\6&1f2e3d4c&0&0008\r\n" +
		"\r\n" +
		"ROOT\\BASICDISPLAY\\0000\r\n" +
		"WARNING: the driver store is being updated\r\n" +
		"  PCI\\VEN_8086&DEV_56A0&SUBSYS_0000&REV_00\\4&2b3c4d5e&0&0008  \r\n"
	ids := deviceIDs(out)
	if len(ids) != 3 {
		t.Fatalf("got %d ids: %q", len(ids), ids)
	}
	if ids[0] != `PCI\VEN_1002&DEV_73BF&SUBSYS_1234&REV_C1\6&1f2e3d4c&0&0008` {
		t.Errorf("first id: %q", ids[0])
	}
	if ids[1] != `ROOT\BASICDISPLAY\0000` {
		t.Errorf("second id: %q", ids[1])
	}
	if ids[2] != `PCI\VEN_8086&DEV_56A0&SUBSYS_0000&REV_00\4&2b3c4d5e&0&0008` {
		t.Errorf("indented id: %q", ids[2])
	}
	if len(deviceIDs("")) != 0 {
		t.Error("nothing in, nothing out")
	}
}

// The same account is spelled with different domains depending on the API:
// a local profile, a Microsoft account and an Azure AD one must all compare
// equal, and two people must not.
func TestSameUser(t *testing.T) {
	same := [][2]string{
		{`LAPTOP\patrick`, `LAPTOP\patrick`},
		{`LAPTOP\Patrick`, `laptop\patrick`},
		{`LAPTOP\patrick`, `AzureAD\patrick`},
		{`MicrosoftAccount\a@b.com`, `AzureAD\a@b.com`},
		{`AzureAD\a@b.com`, `a@b.com`},
		{`patrick`, `patrick`},
	}
	for _, p := range same {
		if !sameUser(p[0], p[1]) {
			t.Errorf("%q and %q should be the same user", p[0], p[1])
		}
	}
	apart := [][2]string{
		{`LAPTOP\patrick`, `LAPTOP\alex`},
		{`CORP\patrick`, `CORP\patricia`},
		{``, `LAPTOP\patrick`},
		{`LAPTOP\patrick`, ``},
	}
	for _, p := range apart {
		if sameUser(p[0], p[1]) {
			t.Errorf("%q and %q must not be the same user", p[0], p[1])
		}
	}
}

// Reads a program's own description from its version resource, the way the app
// list names Chrome and Discord. Read-only; skipped where the file is missing.
func TestFileDescription(t *testing.T) {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	exe := filepath.Join(root, "System32", "notepad.exe")
	if _, err := os.Stat(exe); err != nil {
		t.Skipf("no %s", exe)
	}
	if s := fileDescription(exe); s == "" {
		t.Errorf("no description read from %s", exe)
	} else {
		t.Logf("%s: %q", exe, s)
	}
	if s := fileDescription(`C:\does\not\exist.exe`); s != "" {
		t.Errorf("a missing file gave %q", s)
	}
}

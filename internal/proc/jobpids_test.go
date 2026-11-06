package proc

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// jobPidsList builds a JOBOBJECT_BASIC_PROCESS_ID_LIST the way Windows
// fills one, for the given pointer size.
func jobPidsList(ptrSize int, ids ...uint32) []byte {
	b := make([]byte, 8+len(ids)*ptrSize)
	binary.LittleEndian.PutUint32(b[0:4], uint32(len(ids)))
	binary.LittleEndian.PutUint32(b[4:8], uint32(len(ids)))
	for i, id := range ids {
		if ptrSize == 8 {
			binary.LittleEndian.PutUint64(b[8+i*ptrSize:], uint64(id))
		} else {
			binary.LittleEndian.PutUint32(b[8+i*ptrSize:], id)
		}
	}
	return b
}

func TestParseJobPids(t *testing.T) {
	for _, ptrSize := range []int{4, 8} {
		got := parseJobPids(jobPidsList(ptrSize, 1234, 5678), ptrSize)
		want := []int{1234, 5678}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("ptrSize %d: got %v, want %v", ptrSize, got, want)
		}
	}
}

// The header is two DWORDs, not two ULONG_PTRs: on 32-bit Windows the
// number of ids is at offset 4 and the first id at offset 8, so reading
// the list as a []uintptr from index 1 returned the count as a pid.
func TestParseJobPids32BitFirstID(t *testing.T) {
	got := parseJobPids(jobPidsList(4, 42), 4)
	if len(got) != 1 || got[0] != 42 {
		t.Fatalf("got %v, want [42]", got)
	}
}

func TestParseJobPidsTruncated(t *testing.T) {
	b := jobPidsList(8, 1, 2, 3)
	// A count larger than the buffer must stop at the buffer, not panic.
	binary.LittleEndian.PutUint32(b[4:8], 99)
	got := parseJobPids(b, 8)
	if len(got) != 3 {
		t.Fatalf("got %v, want three ids", got)
	}
	if got := parseJobPids(b[:7], 8); got != nil {
		t.Fatalf("short header: got %v", got)
	}
	if got := parseJobPids(b, 3); got != nil {
		t.Fatalf("bad pointer size: got %v", got)
	}
}

func TestParseJobPidsEmpty(t *testing.T) {
	if got := parseJobPids(jobPidsList(8), 8); len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}

func TestParseJobPidsLarge(t *testing.T) {
	ids := make([]uint32, 300)
	for i := range ids {
		ids[i] = uint32(1000 + i)
	}
	got := parseJobPids(jobPidsList(8, ids...), 8)
	if len(got) != 300 || got[299] != 1299 {
		t.Fatalf("got %d ids, last %v", len(got), got[len(got)-1])
	}
	if !bytes.Equal(jobPidsList(8, ids...)[:8], []byte{44, 1, 0, 0, 44, 1, 0, 0}) {
		t.Fatal("fixture header changed")
	}
}

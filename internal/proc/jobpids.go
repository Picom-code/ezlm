package proc

import "encoding/binary"

// jobPidsSize is a starting buffer for JOBOBJECT_BASIC_PROCESS_ID_LIST: the
// 8-byte header and room for 256 process ids (the common case).
const jobPidsSize = 8 + 256*8

// parseJobPids decodes a JOBOBJECT_BASIC_PROCESS_ID_LIST:
//
//	DWORD     NumberOfAssignedProcesses;   // offset 0
//	DWORD     NumberOfProcessIdsInList;    // offset 4
//	ULONG_PTR ProcessIdList[];             // offset 8, 4 or 8 bytes each
//
// The ids always start at byte 8 (the header is two DWORDs, so it is also
// naturally aligned for a 64-bit ULONG_PTR); ptrSize is unsafe.Sizeof of a
// uintptr on the machine that filled the buffer.
func parseJobPids(b []byte, ptrSize int) []int {
	if len(b) < 8 || (ptrSize != 4 && ptrSize != 8) {
		return nil
	}
	n := int(binary.LittleEndian.Uint32(b[4:8]))
	var out []int
	for i := 0; i < n; i++ {
		off := 8 + i*ptrSize
		if off+ptrSize > len(b) {
			break
		}
		if ptrSize == 8 {
			out = append(out, int(binary.LittleEndian.Uint64(b[off:])))
		} else {
			out = append(out, int(binary.LittleEndian.Uint32(b[off:])))
		}
	}
	return out
}

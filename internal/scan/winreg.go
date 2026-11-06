package scan

import (
	"encoding/binary"
	"strings"
)

// regValue is one value from a display driver's registry entry: a number
// (REG_DWORD or REG_QWORD) or raw bytes (REG_BINARY). Drivers disagree on
// which type holds the card's memory, so both forms are kept.
type regValue struct {
	num   uint64
	bin   []byte
	isNum bool
}

// number is the value as a number. Binary values are little-endian, 8 bytes
// (a QWORD written by hand) or 4 (the older form, which caps at 4 GB).
func (v regValue) number() (uint64, bool) {
	if v.isNum {
		return v.num, true
	}
	switch len(v.bin) {
	case 8:
		return binary.LittleEndian.Uint64(v.bin), true
	case 4:
		return uint64(binary.LittleEndian.Uint32(v.bin)), true
	}
	return 0, false
}

// adapterVRAM is a card's own memory in bytes, from the two values its
// driver entry carries: HardwareInformation.qwMemorySize (the real size) and
// HardwareInformation.MemorySize (older, 4 bytes, so at most 4 GB).
func adapterVRAM(qwMemorySize, memorySize regValue) uint64 {
	if n, ok := qwMemorySize.number(); ok {
		return n
	}
	if n, ok := memorySize.number(); ok {
		return n
	}
	return 0
}

// deviceIDs reads the PNP device IDs a PowerShell command printed, one per
// line. A device instance ID has no spaces, so a line that has one is a
// message in the system language, not a device.
func deviceIDs(out string) []string {
	var ids []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.ContainsAny(line, " \t") {
			continue
		}
		ids = append(ids, line)
	}
	return ids
}

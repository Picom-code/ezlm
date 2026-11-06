package scan

import "encoding/binary"

// registryValues is the part of a display driver's registry entry that the
// scan reads. On Windows a registry.Key has these methods; tests use a map.
type registryValues interface {
	GetStringValue(name string) (string, uint32, error)
	GetIntegerValue(name string) (uint64, uint32, error)
	GetBinaryValue(name string) ([]byte, uint32, error)
}

// adapterFrom reads one display driver entry ("0001"). ok is false when the
// entry names no card, which is how stale and non-display entries look.
func adapterFrom(k registryValues) (displayAdapter, bool) {
	var a displayAdapter
	a.Name, _, _ = k.GetStringValue("DriverDesc")
	a.Provider, _, _ = k.GetStringValue("ProviderName")
	a.VRAM = adapterVRAMFromKey(k)
	return a, a.Name != ""
}

// adapterVRAM reads the card's memory from its driver entry. Current drivers
// store it in the 64-bit qwMemorySize; older ones in MemorySize, either as a
// number or as a little-endian byte string.
func adapterVRAMFromKey(k registryValues) uint64 {
	if v, _, err := k.GetIntegerValue("HardwareInformation.qwMemorySize"); err == nil && v != 0 {
		return v
	}
	if b, _, err := k.GetBinaryValue("HardwareInformation.MemorySize"); err == nil {
		switch {
		case len(b) >= 8: // some drivers write the size as a 64-bit value
			return binary.LittleEndian.Uint64(b)
		case len(b) >= 4: // older drivers: 4 bytes, so at most 4 GB
			return uint64(binary.LittleEndian.Uint32(b))
		}
	}
	if v, _, err := k.GetIntegerValue("HardwareInformation.MemorySize"); err == nil {
		return v
	}
	return 0
}

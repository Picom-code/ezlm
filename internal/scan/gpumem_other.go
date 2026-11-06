//go:build !windows

package scan

// otherGPUMemory: only Windows has AMD and Intel cards without a vendor tool
// that QuickMemory reads (Macs share memory; Linux AMD cards have rocm-smi).
func otherGPUMemory() (total, free uint64) { return 0, 0 }

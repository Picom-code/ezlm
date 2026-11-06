//go:build windows

package scan

import (
	"context"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const displayClass = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`

// displayScan finds AMD and Intel graphics cards on Windows, where there's
// no rocm-smi: their display drivers' entries give each card's name and
// graphics memory, and the per-app numbers (as for NVIDIA cards) say how
// much of it other apps hold.
func displayScan(ctx context.Context, h *Hardware) bool {
	gpus := usableAdapters(displayAdapters(ctx))
	if len(gpus) == 0 {
		return false
	}
	h.GPUs = gpus
	h.Backend = Vulkan
	otherGPU.set(gpus[0].MemTotal)
	var used uint64
	for _, b := range windowsGPUMem(ctx, h) {
		used += b
	}
	// Which card each app's memory is on isn't known here, so take it from
	// the cards in turn: what's free in total comes out right.
	for i := range h.GPUs {
		g := &h.GPUs[i]
		take := min(used, g.MemTotal)
		g.MemFree = g.MemTotal - take
		used -= take
	}
	return true
}

// displayAdapters lists the graphics cards in this PC. The driver entries
// also remember cards that were taken out, so only the ones Windows reports
// as present count (each points to its entry through its "Driver" value).
func displayAdapters(ctx context.Context) []displayAdapter {
	out, err := runPS(ctx,
		`Get-CimInstance Win32_VideoController -ErrorAction SilentlyContinue | ForEach-Object { $_.PNPDeviceID }`)
	if err != nil {
		return nil
	}
	var list []displayAdapter
	for _, id := range deviceIDs(out) {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Enum\`+id, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		drv, _, err := k.GetStringValue("Driver") // "{4d36e968-...}\0001"
		k.Close()
		i := strings.LastIndex(drv, `\`)
		if err != nil || i < 0 {
			continue
		}
		if a, ok := readAdapter(drv[i+1:]); ok {
			list = append(list, a)
		}
	}
	return list
}

// deviceIDs picks the device instance paths ("PCI\VEN_1002&...") out of what
// PowerShell prints, one per line. Anything else it prints (a warning, a
// blank line) is not a path and is dropped, so a stray line can't become a
// card. A byte order mark on the first line is dropped too.
// readAdapter reads one display driver entry ("0001").
func readAdapter(n string) (displayAdapter, bool) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, displayClass+`\`+n, registry.QUERY_VALUE)
	if err != nil {
		return displayAdapter{}, false
	}
	defer k.Close()
	var a displayAdapter
	a.Name, _, _ = k.GetStringValue("DriverDesc")
	a.Provider, _, _ = k.GetStringValue("ProviderName")
	var qw, ms regValue
	if v, _, err := k.GetIntegerValue("HardwareInformation.qwMemorySize"); err == nil {
		qw = regValue{num: v, isNum: true}
	}
	if b, _, err := k.GetBinaryValue("HardwareInformation.MemorySize"); err == nil {
		ms = regValue{bin: b}
	}
	a.VRAM = adapterVRAM(qw, ms)
	return a, a.Name != ""
}

// adapterVRAM picks the card's memory from the values its driver writes: the
// 64-bit size, an older binary one, or a DWORD. A value that is there but
// zero counts as missing, so a driver that leaves one empty still reports its
// card.
// otherGPU is the AMD or Intel card models run on, for live dashboards: its
// size comes from the full scan (or a one-time look in the background).
var otherGPU cardSize

type cardSize struct {
	sync.Mutex
	total uint64
	asked bool
}

func (o *cardSize) set(total uint64) {
	o.Lock()
	o.total, o.asked = total, true
	o.Unlock()
}

// otherGPUMemory: the card's memory and how much is free right now. In use is
// read from the graphics memory counters, taking the busiest adapter that
// fits the card (the built-in graphics beside it hold little of their own).
func otherGPUMemory() (total, free uint64) {
	otherGPU.Lock()
	total, asked := otherGPU.total, otherGPU.asked
	otherGPU.asked = true
	otherGPU.Unlock()
	if !asked { // no scan yet in this session: find the card once, without holding up the dashboard
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if g := usableAdapters(displayAdapters(ctx)); len(g) > 0 {
				otherGPU.set(g[0].MemTotal)
			}
		}()
	}
	if total == 0 {
		return 0, 0
	}
	var used uint64
	for _, u := range adapterMemoryUsed() {
		if u <= total && u > used {
			used = u
		}
	}
	return total, total - used
}

var (
	pdh                  = windows.NewLazySystemDLL("pdh.dll")
	pdhOpenQuery         = pdh.NewProc("PdhOpenQueryW")
	pdhAddEnglishCounter = pdh.NewProc("PdhAddEnglishCounterW")
	pdhCollectQueryData  = pdh.NewProc("PdhCollectQueryData")
	pdhGetFormattedArray = pdh.NewProc("PdhGetFormattedCounterArrayW")
	pdhCloseQuery        = pdh.NewProc("PdhCloseQuery")
)

// adapterMemoryUsed reads each graphics adapter's memory in use ("GPU Adapter
// Memory\Dedicated Usage", by adapter), straight from Windows' performance
// counters: milliseconds, no PowerShell, and the English counter names work
// on Windows in any language. Nil if the counters can't be read.
func adapterMemoryUsed() map[string]uint64 {
	const fmtLarge, moreData = 0x400, 0x800007D2
	if pdhOpenQuery.Find() != nil {
		return nil
	}
	var q uintptr
	if r, _, _ := pdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&q))); r != 0 {
		return nil
	}
	defer pdhCloseQuery.Call(q)
	path, _ := windows.UTF16PtrFromString(`\GPU Adapter Memory(*)\Dedicated Usage`)
	var c uintptr
	if r, _, _ := pdhAddEnglishCounter.Call(q, uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&c))); r != 0 {
		return nil
	}
	if r, _, _ := pdhCollectQueryData.Call(q); r != 0 {
		return nil
	}
	var size, count uint32
	if r, _, _ := pdhGetFormattedArray.Call(c, fmtLarge, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0); uint32(r) != moreData || size == 0 {
		return nil
	}
	buf := make([]byte, size)
	if r, _, _ := pdhGetFormattedArray.Call(c, fmtLarge, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0]))); r != 0 {
		return nil
	}
	// PDH_FMT_COUNTERVALUE_ITEM_W: the instance name, then the value's status and the value.
	type item struct {
		name   *uint16
		status uint32
		_      uint32
		value  int64
	}
	used := map[string]uint64{}
	for _, it := range unsafe.Slice((*item)(unsafe.Pointer(&buf[0])), count) {
		if it.status <= 1 && it.value >= 0 { // valid or new data
			used[windows.UTF16PtrToString(it.name)] += uint64(it.value)
		}
	}
	return used
}

package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/picom-code/ezlm/internal/report"
	"github.com/picom-code/ezlm/internal/update"
	"github.com/picom-code/ezlm/internal/version"
	"math"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/picom-code/ezlm/internal/bench"
	"github.com/picom-code/ezlm/internal/catalog"
	"github.com/picom-code/ezlm/internal/fit"
	"github.com/picom-code/ezlm/internal/integrate"
	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/serve"
)

// Demo is a scripted backend for walkthroughs and UI work: nothing is
// installed. Scan results are computed by the real fit engine for an RTX 4070
// SUPER desktop whose graphics card starts out busy with another program.
type Demo struct {
	bus
	mu        sync.Mutex
	gpuBusy   bool
	deployed  *serve.Config
	apps      map[string]*demoApp // connectable apps, by id (see demoApps)
	busyJob   string
	pref      fit.Pref
	cancelled bool
	removed   map[string]bool
	load      *demoLoad // a model "loading" right now (drives the memory animation)
	closed    map[string]bool
}

type demoLoad struct {
	at       time.Time
	gpu, ram uint64
}

func (d *Demo) Cancel() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.busyJob == "" {
		return fmt.Errorf("nothing is running")
	}
	d.cancelled = true
	return nil
}

func (d *Demo) stopped() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cancelled
}

func NewDemo() *Demo {
	return &Demo{gpuBusy: true, apps: demoApps()}
}

const (
	G = scan.GB
	M = scan.GB / 1024
)

func demoHW(gpuBusy bool) *scan.Hardware {
	free := uint64(11*G + 7*G/10)
	procs := []scan.Proc{{PID: 25684, Name: "obs64.exe", Mem: 300 << 20, Killable: true}}
	avail := uint64(42 * G)
	if gpuBusy {
		free, avail = G/5, 31*G
		procs = append([]scan.Proc{{PID: 7440, Name: "llama-server.exe", Mem: 11*G + G/2, Killable: true}}, procs...)
	}
	return &scan.Hardware{
		OS: "windows", OSName: "Windows 11 Pro 10.0.26200", Arch: "amd64",
		CPUName: "AMD Ryzen 7 9800X3D 8-Core Processor", Cores: 16,
		RAMTotal: 62 * G, RAMAvail: avail, DiskFree: 101 * G, ModelsDir: `D:\EZLM\models`,
		Backend: scan.CUDA, GPUUsable: free, CPUBandwidth: 60, GPUProcs: procs,
		GPUs: []scan.GPU{{Name: "NVIDIA GeForce RTX 4070 SUPER", Vendor: "nvidia", MemTotal: 12 * G, MemFree: free, Bandwidth: 504, Driver: "616.92"}},
	}
}

func demoResult(hw *scan.Hardware, c *catalog.Catalog, pref fit.Pref) *ScanResult {
	r := &ScanResult{HW: hw, CatalogSource: "embedded", Pref: string(pref)}
	r.Plans = fit.PlansFor(hw, c, fit.ComputeBudgets(hw, hw.RAMAvail), pref)
	if p, ok := fit.Recommend(r.Plans); ok {
		r.Recommended = &p
	}
	return r
}

func (d *Demo) Scan(ctx context.Context, pref string) *ScanResult {
	pf := fit.Pref(pref)
	if pf != fit.Smartest && pf != fit.Fastest {
		pf = fit.Balanced
	}
	time.Sleep(600 * time.Millisecond)
	c, _ := catalog.Load(ctx)
	d.mu.Lock()
	busy := d.gpuBusy
	d.mu.Unlock()
	r := demoResult(demoHW(busy), c, pf)
	d.mu.Lock()
	d.pref = pf
	d.mu.Unlock()
	if busy {
		if f := demoResult(demoHW(false), c, pf); f.Recommended != nil {
			r.Unlock, r.UnlockFreeGB, r.UnlockGPU = f.Recommended, 11.5, true
		}
	}
	return r
}

func (d *Demo) Status(context.Context) Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := Status{Busy: d.busyJob, Targets: d.targets()}
	if d.deployed != nil {
		c := *d.deployed
		c.Keys = append([]serve.APIKey(nil), d.deployed.Keys...)
		s.Deployed, s.Healthy, s.Config, s.LocalURL = true, true, &c, c.LocalURL()
		if c.LAN {
			s.LANURL = fmt.Sprintf("http://192.168.1.42:%d/v1", c.Port)
		}
	}
	return s
}

func (d *Demo) Kill(context.Context, int32) error {
	time.Sleep(700 * time.Millisecond)
	d.mu.Lock()
	d.gpuBusy = false
	d.mu.Unlock()
	return nil
}

// job runs fn in the background, like App.start.
func (d *Demo) job(name string, fn func(ev func(Event))) error {
	return d.jobErr(name, func(ev func(Event)) error { fn(ev); return nil })
}

// jobErr is job for scripts that can fail: the error ends the job the way a
// real failure does.
func (d *Demo) jobErr(name string, fn func(ev func(Event)) error) error {
	d.mu.Lock()
	if d.busyJob != "" {
		d.mu.Unlock()
		return ErrBusy
	}
	d.busyJob, d.cancelled = name, false
	d.mu.Unlock()
	d.resetHistory()
	go func() {
		err := fn(func(e Event) {
			if !d.stopped() {
				e.Job = name
				d.emit(e)
			}
		})
		d.mu.Lock()
		d.busyJob = ""
		c := d.cancelled
		d.mu.Unlock()
		if c {
			d.emit(Event{Job: name, Step: "done", State: "cancelled", Msg: "Cancelled"})
			return
		}
		if err != nil {
			d.emit(Event{Job: name, Step: "done", State: "fail", Msg: err.Error()})
			return
		}
		d.emit(Event{Job: name, Step: "done", State: "ok", Data: d.Status(context.Background())})
	}()
	return nil
}

func (d *Demo) Deploy(modelID string) error {
	d.mu.Lock()
	pf := d.pref
	d.mu.Unlock()
	r := d.Scan(context.Background(), string(pf))
	p := r.Recommended
	for i := range r.Plans {
		if r.Plans[i].Model.ID == modelID {
			p = &r.Plans[i]
		}
	}
	if p == nil {
		return fmt.Errorf("no model fits")
	}
	plan := *p
	return d.job("deploy", func(ev func(Event)) {
		step := func(s, state, msg string) { ev(Event{Step: s, State: state, Msg: msg}) }
		bytes := func(s string, total uint64, secs int, bps float64) {
			for t := 0; t <= secs*10 && !d.stopped(); t++ {
				ev(Event{Step: s, State: "progress", Done: total * uint64(t) / uint64(secs*10), Total: total, BPS: bps})
				d.nap(100 * time.Millisecond)
			}
		}
		step("engine", "run", "Getting the AI engine (llama.cpp) for your cuda hardware")
		bytes("engine", 450<<20, 2, 48e6)
		step("engine", "ok", "Engine ready (b11960)")
		step("model", "run", fmt.Sprintf("Downloading %s (%.1f GB)", plan.Model.Name, float64(plan.Quant.Bytes)/float64(G)))
		bytes("model", plan.Quant.Bytes, 9, 112e6)
		step("model", "ok", plan.Model.Name+" downloaded")
		step("start", "run", "Starting your server")
		d.mu.Lock()
		prev := d.deployed
		d.load = &demoLoad{at: time.Now(), gpu: plan.GPUNeed, ram: plan.RAMNeed}
		d.mu.Unlock()
		if plan.NCPUMoE > 0 {
			step("start", "info", "Reading the model into memory first, so your first questions are fast")
			for i := 0; i <= 12; i++ {
				ev(Event{Step: "load", State: "progress", Done: plan.Quant.Bytes * uint64(i) / 12, Total: plan.Quant.Bytes})
				d.nap(180 * time.Millisecond)
			}
		}
		step("start", "ok", "Server registered on port 8080")
		step("load", "run", "Loading the model into memory")
		for s := 0; s <= 14; s++ {
			ev(Event{Step: "load", State: "progress", Msg: fmt.Sprintf("%ds", s)})
			d.nap(250 * time.Millisecond)
		}
		d.mu.Lock()
		d.load = nil
		d.mu.Unlock()
		step("load", "ok", "Model loaded")
		step("test", "run", "Asking your model a question")
		d.nap(1600 * time.Millisecond)
		step("test", "ok", "Hello! I'm Qwen, an AI model running entirely on your computer.")
		if d.stopped() {
			d.mu.Lock()
			d.deployed, d.load = prev, nil // cancelled: the previous model is back
			d.mu.Unlock()
			return
		}
		k := demoKey()
		d.mu.Lock()
		d.deployed = &serve.Config{Key: k, Keys: []serve.APIKey{{ID: "k1", Name: "Default", Key: k, Created: time.Now()}},
			Port: 8080, ModelID: plan.Model.ID, ModelName: plan.Model.Name, Quant: plan.Quant.Name, Ctx: plan.Ctx,
			NGL: plan.NGL, NCPUMoE: plan.NCPUMoE, ToolCalls: plan.Model.ToolCalling, DeployedAt: time.Now()}
		d.mu.Unlock()
	})
}

func (d *Demo) SetLAN(on bool) error {
	return d.job("lan", func(ev func(Event)) {
		ev(Event{Step: "lan", State: "run", Msg: "Restarting the server"})
		time.Sleep(1200 * time.Millisecond)
		d.mu.Lock()
		if d.deployed != nil {
			d.deployed.LAN = on
		}
		d.mu.Unlock()
	})
}

func (d *Demo) Integrate(ids []string, _ bool) error {
	return d.job("integrate", func(ev func(Event)) {
		for _, id := range ids {
			d.mu.Lock()
			da := d.apps[id]
			d.mu.Unlock()
			if da == nil {
				continue
			}
			name := demoName(id)
			s := "integrate:" + id
			ev(Event{Step: s, State: "run", Msg: "Setting up " + name})
			if da.state == integrate.StateNeeds64K {
				ev(Event{Step: s, State: "info", Msg: "Hermes needs a 64K context window: restarting your server with more room"})
				d.nap(1800 * time.Millisecond)
			}
			if da.state == integrate.StateNotInstalled {
				ev(Event{Step: s, State: "info", Msg: "Installing " + name + "…"})
				for _, l := range demoInstallLines[id] {
					ev(Event{Step: s, State: "info", Msg: l})
					d.nap(700 * time.Millisecond)
				}
			}
			ev(Event{Step: s, State: "info", Msg: fmt.Sprintf("Adding EZLM as %s %s provider…", map[bool]string{true: "an", false: "a"}[id == "opencode"], name)})
			d.nap(900 * time.Millisecond)
			ev(Event{Step: s, State: "info", Msg: "Testing " + name + " with your model…"})
			d.nap(2600 * time.Millisecond)
			d.mu.Lock()
			da.connect()
			d.mu.Unlock()
			ev(Event{Step: s, State: "ok", Msg: "Ready"})
		}
	})
}

func (d *Demo) keys(what string, edit func(c *serve.Config)) error {
	d.mu.Lock()
	if d.deployed == nil {
		d.mu.Unlock()
		return fmt.Errorf("nothing is deployed yet")
	}
	d.mu.Unlock()
	return d.job("keys", func(ev func(Event)) {
		d.mu.Lock()
		edit(d.deployed)
		d.deployed.Key = d.deployed.Keys[0].Key
		d.mu.Unlock()
		ev(Event{Step: "keys", State: "run", Msg: what + ": restarting the server"})
		time.Sleep(2200 * time.Millisecond)
		ev(Event{Step: "keys", State: "ok", Msg: "Done"})
	})
}

func (d *Demo) AddKey(name string) error {
	return d.keys("Creating a new key", func(c *serve.Config) {
		if name == "" {
			name = fmt.Sprintf("Key %d", len(c.Keys)+1)
		}
		c.Keys = append(c.Keys, serve.APIKey{ID: fmt.Sprintf("k%x", time.Now().UnixNano()&0xffffff), Name: name, Key: demoKey(), Created: time.Now()})
	})
}

func (d *Demo) RotateKey(id string) error {
	return d.keys("Rotating the key", func(c *serve.Config) {
		for i := range c.Keys {
			if c.Keys[i].ID == id {
				c.Keys[i].Key = demoKey()
			}
		}
	})
}

func (d *Demo) RevokeKey(id string) error {
	d.mu.Lock()
	n := 0
	if d.deployed != nil {
		n = len(d.deployed.Keys)
	}
	d.mu.Unlock()
	if n <= 1 {
		return fmt.Errorf("the server needs at least one key")
	}
	return d.keys("Revoking the key", func(c *serve.Config) {
		for i := range c.Keys {
			if c.Keys[i].ID == id {
				c.Keys = append(c.Keys[:i], c.Keys[i+1:]...)
				return
			}
		}
	})
}

func (d *Demo) Stop() error  { return nil }
func (d *Demo) Start() error { return nil }

func (d *Demo) Uninstall() error {
	d.mu.Lock()
	d.deployed, d.gpuBusy = nil, true
	d.apps = demoApps()
	d.mu.Unlock()
	return nil
}

func demoKey() string {
	const a = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	b := make([]byte, 40)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(a))))
		b[i] = a[n.Int64()]
	}
	return "ezlm-" + string(b)
}

// Live fakes a busy-but-healthy server so dashboards have something to show.
func (d *Demo) Live(context.Context) Live {
	d.mu.Lock()
	defer d.mu.Unlock()
	t := float64(time.Now().UnixMilli()%60000) / 60000
	l := Live{RAMTotal: 62 * G, RAMAvail: 42 * G, GPUTotal: 12 * G, GPUFree: 11*G + 7*G/10}
	if d.gpuBusy {
		l.GPUFree, l.RAMAvail = G/5, 31*G
	}
	if ld := d.load; ld != nil {
		// RAM fills with the pre-read; graphics memory as the server uploads.
		el := time.Since(ld.at).Seconds()
		ram := math.Min(1, el/2.4)
		gpu := math.Max(0, math.Min(1, (el-2.4)/3.4))
		l.Starting, l.Phase = true, "idle"
		l.GPUPlan, l.RAMPlan = ld.gpu, ld.ram
		l.GPUFree -= min(l.GPUFree, uint64(gpu*float64(ld.gpu)))
		l.RAMAvail -= min(l.RAMAvail, uint64(ram*float64(ld.ram)))
		l.LoadDone, l.LoadTotal = uint64(ram*1000), 1000
		return l
	}
	if d.deployed != nil {
		l.Healthy = true
		l.GPUFree = 1*G + 6*G/10
		l.RAMAvail = 25 * G
		wave := 0.5 + 0.5*sinApprox(t*12)
		l.Phase = "idle"
		if wave > 0.35 {
			l.Requests = 1
			l.TokPerSec = 26 + 8*wave
			if wave < 0.55 {
				l.Phase, l.PromptTotal, l.PromptDone = "reading", 8192, int((wave-0.35)/0.2*8192)
			} else {
				l.Phase, l.PromptTotal, l.PromptDone, l.Written = "writing", 8192, 8192, int((wave-0.55)*900)
			}
		}
		l.Tokens = uint64(time.Since(d.deployed.DeployedAt).Seconds() * 14)
		l.Uptime = int64(time.Since(d.deployed.DeployedAt).Seconds())
	}
	return l
}

func (d *Demo) Logs(int) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.deployed == nil {
		return []string{}
	}
	return []string{
		"main: server is listening on http://127.0.0.1:8080",
		"load_tensors: offloaded 41/41 layers to GPU",
		"load_tensors: CUDA0 model buffer size =  9216.00 MiB",
		"load_tensors: CPU_Mapped model buffer size = 17408.00 MiB",
		"llama_context: n_ctx = 65536, flash_attn = auto",
		"srv  update_slots: all slots are idle",
		"slot launch_slot_: id 0 | task 12 | processing task",
		"slot print_timing: id 0 | task 12 | prompt eval time = 412.33 ms / 1832 tokens (4443 tok/s)",
		"slot print_timing: id 0 | task 12 |        eval time = 9881.02 ms /  301 tokens (30.46 tok/s)",
		"srv  update_slots: all slots are idle",
	}
}

func sinApprox(x float64) float64 {
	// Bhaskara-style approximation is plenty for a demo wave.
	x = x - float64(int(x/6.283185))*6.283185
	if x > 3.141593 {
		return -sinApprox(x - 3.141593)
	}
	return 16 * x * (3.141593 - x) / (49.348 - 4*x*(3.141593-x))
}

func (d *Demo) Test() error {
	return d.job("test", func(ev func(Event)) {
		results := []bench.Result{
			{Name: "Speed", Pass: true, Detail: "31.4 tokens/s writing (600 tokens)", Value: 31.4, Secs: 19.8},
			{Name: "Reasoning", Pass: true, Detail: "bat-and-ball: answered 5 (correct: 5)", Secs: 11.2},
			{Name: "Tool calling", Pass: true, Detail: `called get_weather({"city":"Austin"})`, Secs: 2.9},
			{Name: "Following instructions", Pass: true, Detail: `strict JSON: {"name": "Neil Armstrong", "year": 1969}`, Secs: 6.1},
			{Name: "Long-context recall", Pass: true, Detail: "found it in 2687 tokens, read at 1840 tokens/s: 7342", Secs: 3.4},
		}
		for _, r := range results {
			time.Sleep(time.Duration(r.Secs*120) * time.Millisecond)
			ev(Event{Step: "result", State: "ok", Msg: r.Name, Data: r})
		}
	})
}

// nap sleeps, returning early if the job was cancelled.
func (d *Demo) nap(t time.Duration) {
	for end := time.Now().Add(t); time.Now().Before(end) && !d.stopped(); {
		time.Sleep(25 * time.Millisecond)
	}
}

// Installed (demo): the running model plus one other already downloaded.
func (d *Demo) Installed(ctx context.Context, pref string) []InstalledModel {
	r := d.Scan(ctx, pref)
	d.mu.Lock()
	running := ""
	if d.deployed != nil {
		running = d.deployed.ModelID
	}
	removed := d.removed
	d.mu.Unlock()
	var out []InstalledModel
	for i, p := range r.Plans {
		if i > 2 {
			break
		}
		if p.Model.ID != running && removed[p.Model.ID] {
			continue
		}
		if p.Model.ID != running && i > 1 {
			continue
		}
		pp := p
		out = append(out, InstalledModel{ModelID: p.Model.ID, Name: p.Model.Name, Vendor: p.Model.Vendor, Country: p.Model.Country,
			Quant: p.Quant.Name, Bytes: p.Quant.Bytes, Running: p.Model.ID == running, Plan: &pp})
	}
	return out
}

// Use (demo): switching to a downloaded model skips the download.
func (d *Demo) Use(ctx context.Context, modelID, quant, pref string) error {
	return d.Deploy(modelID)
}

func (d *Demo) Remove(ctx context.Context, modelID, quant string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.deployed != nil && d.deployed.ModelID == modelID {
		return fmt.Errorf("that's the model you're running; switch to another one first")
	}
	if d.removed == nil {
		d.removed = map[string]bool{}
	}
	d.removed[modelID] = true
	return nil
}

// Model mode in the demo: a believable desktop, and closing always works
// (Discord pretends to hide in the tray first, to show the force option).
func (d *Demo) Apps(context.Context) []scan.App {
	d.mu.Lock()
	defer d.mu.Unlock()
	all := []scan.App{
		{ID: "chrome", Name: "Google Chrome", PIDs: []int32{101, 102, 103}, RAM: 1900 * M, GPU: 420 * M, Windows: true, Kind: "app", Note: "Google Chrome reopens your tabs next time."},
		{ID: "steam", Name: "Steam", PIDs: []int32{201, 202}, RAM: 640 * M, GPU: 180 * M, Windows: true, Kind: "app", Note: "Closing Steam also pauses downloads and updates."},
		{ID: "discord", Name: "Discord", PIDs: []int32{301, 302}, RAM: 520 * M, GPU: 90 * M, Windows: true, Kind: "app", Note: "Discord keeps running in the background after its window closes; EZLM will offer to close it fully."},
		{ID: "ollama", Name: "Ollama", PIDs: []int32{401}, RAM: 300 * M, GPU: 2900 * M, Windows: false, Kind: "ai", Note: "Another AI model server, not EZLM's. Close it only if you're not using it."},
		{ID: "spotify", Name: "Spotify", PIDs: []int32{501}, RAM: 310 * M, Windows: true, Kind: "app", Note: "It may ask you to save your work first."},
	}
	var out []scan.App
	for _, a := range all {
		if !d.closed[a.ID] {
			out = append(out, a)
		}
	}
	return out
}

func (d *Demo) CloseApp(ctx context.Context, id string, force bool) (scan.CloseResult, error) {
	d.nap(1400 * time.Millisecond)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed == nil {
		d.closed = map[string]bool{}
	}
	if (id == "discord" || id == "ollama") && !force {
		return scan.CloseResult{Remaining: 1, NeedForce: true}, nil
	}
	freed := map[string]uint64{"chrome": 1900 * M, "steam": 640 * M, "discord": 520 * M, "ollama": 300 * M, "spotify": 310 * M}[id]
	d.closed[id] = true
	return scan.CloseResult{Closed: true, Freed: freed}, nil
}

// Updates and storage in the demo: an update is "available" and downloads
// for show; storage lists two drives and pretends to move.
func (d *Demo) UpdateInfo(context.Context, bool) update.Info {
	return update.Info{Current: version.Version, Available: true, Checked: time.Now(),
		Latest: &update.Release{Version: "0.1.2-beta", Name: "EZLM 0.1.2 beta", Notes: "Faster model switching and fixes.", Asset: "EZLM-Setup.exe", Size: 123 << 20}}
}

func (d *Demo) DownloadUpdate() error {
	return d.job("update", func(ev func(Event)) {
		step := func(st, state, msg string) { ev(Event{Step: st, State: state, Msg: msg}) }
		step("download", "run", "Downloading EZLM 0.1.2-beta")
		for i := 0; i <= 20; i++ {
			ev(Event{Step: "download", State: "progress", Done: uint64(i) * (123 << 20) / 20, Total: 123 << 20, BPS: 40e6})
			d.nap(120 * time.Millisecond)
		}
		step("download", "ok", "(demo: nothing was downloaded)")
	})
}

func (d *Demo) Storage(context.Context) StorageInfo {
	return StorageInfo{Dir: `D:\EZLM\models`, Free: 412 * G, ModelsBytes: 63 * G, Drives: []Drive{
		{Path: `D:\`, Label: "Data", Free: 412 * G, Total: 931 * G, Current: true},
		{Path: `C:\`, Label: "Windows", Free: 88 * G, Total: 476 * G},
	}}
}

func (d *Demo) SetStorage(dir string, move bool) error {
	return d.job("storage", func(ev func(Event)) {
		step := func(st, state, msg string) { ev(Event{Step: st, State: state, Msg: msg}) }
		if move {
			for i := 0; i <= 20; i++ {
				ev(Event{Step: "move", State: "progress", Done: uint64(i) * 63 * G / 20, Total: 63 * G})
				d.nap(100 * time.Millisecond)
			}
		}
		step("storage", "ok", "New downloads will go to "+StorageDir(dir)+" (demo)")
	})
}

// Launch and Report in the demo: pretend, never touch real apps or GitHub.
// Hermes refuses while it needs 64K, and DeepSeek Harness fails its first
// launch (exit code 1) so the failure UI can be seen; the next one works.
func (d *Demo) Launch(id, mode string) error {
	d.mu.Lock()
	da := d.apps[id]
	d.mu.Unlock()
	if da == nil {
		return fmt.Errorf("unknown app %q", id)
	}
	name := demoName(id)
	return d.jobErr("launch", func(ev func(Event)) error {
		step := func(s, state, msg string) { ev(Event{Step: s, State: state, Msg: msg}) }
		d.mu.Lock()
		state, failNext := da.state, da.failNext
		model := "your model"
		if d.deployed != nil {
			model = d.deployed.ModelName
		}
		d.mu.Unlock()
		if state == integrate.StateNeeds64K || state == integrate.StateNeedsTools {
			d.mu.Lock()
			err := errors.New(da.detail)
			d.mu.Unlock()
			step("open", "fail", err.Error())
			return err
		}
		if state == integrate.StateNeedsUpdate {
			step("open", "info", fmt.Sprintf("Updating %s to use %s", name, model))
			d.nap(700 * time.Millisecond)
		}
		if state == integrate.StateNotInstalled || state == integrate.StateInstalled {
			step("connect", "run", "Setting up "+name+" (only the first time)")
			d.nap(2500 * time.Millisecond)
			d.mu.Lock()
			da.connect()
			d.mu.Unlock()
			step("connect", "ok", name+" is connected")
		}
		step("open", "run", "Opening "+name)
		d.nap(900 * time.Millisecond)
		if failNext {
			err := fmt.Errorf("%s closed right after opening (exit code 1). Press Report a problem to send the details.", name)
			d.mu.Lock()
			da.failNext = false
			da.state, da.detail = integrate.StateBroken, err.Error()
			da.check = &integrate.CheckResult{At: time.Now(), Stage: integrate.StageLaunch, Msg: err.Error(), LogPath: da.log}
			d.mu.Unlock()
			step("open", "fail", err.Error())
			return err
		}
		d.mu.Lock()
		da.connect()
		d.mu.Unlock()
		where := "in a new window"
		if mode == integrate.ModeWeb || (mode == "" && id == "dsh") {
			where = "in your browser"
		}
		step("open", "ok", fmt.Sprintf("%s is open %s, using %s (demo: nothing was opened)", name, where, model))
		return nil
	})
}

// IntegrationLog (demo): a believable redacted log.
func (d *Demo) IntegrationLog(id string, n int) []string {
	d.mu.Lock()
	da := d.apps[id]
	d.mu.Unlock()
	if da == nil || da.bin == "" {
		return []string{} // never set up: nothing logged yet
	}
	name, article := demoName(id), "a"
	if strings.ContainsRune("AEIOU", rune(name[0])) {
		article = "an"
	}
	lines := []string{
		"2026-11-05T21:02:11Z setup " + id + ": model qwen3.5-9b, context 65536, http://127.0.0.1:8080/v1",
		"2026-11-05T21:02:11Z using " + da.bin + " (version \"" + da.version + "\", source " + da.source + ")",
		"2026-11-05T21:02:12Z Adding EZLM as " + article + " " + name + " provider…",
		"2026-11-05T21:02:12Z Testing " + name + " with your model…",
		"2026-11-05T21:02:26Z exit after 14.2 s: ok",
		"2026-11-05T21:02:26Z setup ok: answered and used a tool (14 s)",
	}
	if da.check != nil && !da.check.OK {
		lines = append(lines, "2026-11-05T21:09:40Z open "+id+": exit code 1", "2026-11-05T21:09:40Z "+da.check.Msg)
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

func (d *Demo) Report(context.Context, string, bool, string) (report.Result, error) {
	d.nap(700 * time.Millisecond)
	return report.Result{Sent: true, URL: "https://github.com/picom-code/ezlm/issues", Number: 1}, nil
}

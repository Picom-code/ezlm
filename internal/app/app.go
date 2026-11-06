// Package app orchestrates scan → pick → install → serve → verify → integrate.
// It has no UI; the web page and the CLI both drive it and listen to Events.
package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/picom-code/ezlm/internal/awake"
	"github.com/picom-code/ezlm/internal/bench"
	"github.com/picom-code/ezlm/internal/catalog"
	"github.com/picom-code/ezlm/internal/engine"
	"github.com/picom-code/ezlm/internal/fit"
	"github.com/picom-code/ezlm/internal/integrate"
	"github.com/picom-code/ezlm/internal/keys"
	"github.com/picom-code/ezlm/internal/paths"
	"github.com/picom-code/ezlm/internal/prereq"
	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/serve"
	"github.com/picom-code/ezlm/internal/version"
	"github.com/shirou/gopsutil/v4/disk"
)

// Event is one line of progress. Steps: engine, model, start, load, test, integrate:<id>, lan, done.
type Event struct {
	Job   string  `json:"job"`
	Step  string  `json:"step"`
	State string  `json:"state"` // run | ok | fail | progress | info
	Msg   string  `json:"msg"`
	Done  uint64  `json:"done,omitempty"`
	Total uint64  `json:"total,omitempty"`
	BPS   float64 `json:"bps,omitempty"`
	Data  any     `json:"data,omitempty"`
}

type ScanResult struct {
	HW          *scan.Hardware `json:"hw"`
	Plans       []fit.Plan     `json:"plans"`
	Recommended *fit.Plan      `json:"recommended"`
	// Unlock is the better model the user could run by closing apps (nil if none).
	Unlock        *fit.Plan      `json:"unlock"`
	UnlockFreeGB  float64        `json:"unlock_free_gb"`
	UnlockGPU     bool           `json:"unlock_gpu"` // the unlock needs graphics memory, not RAM
	CatalogSource string         `json:"catalog_source"`
	Prereqs       []prereq.Check `json:"prereqs"`
	Pref          string         `json:"pref"`
}

type Status struct {
	Deployed bool          `json:"deployed"`
	Healthy  bool          `json:"healthy"`
	Starting bool          `json:"starting"` // server process is up but still loading the model
	Config   *serve.Config `json:"config,omitempty"`
	LocalURL string        `json:"local_url,omitempty"`
	LANURL   string        `json:"lan_url,omitempty"`
	Busy     string        `json:"busy,omitempty"` // name of the running job
	Tune     *Tune         `json:"tune,omitempty"` // a better setup for the running model
	// Hermes64: how the model would run with the 64K context Hermes needs,
	// when it has less (so the app can show the trade-off before connecting).
	Hermes64 *CtxOption `json:"hermes64,omitempty"`
	// Unmanaged: the server answers but EZLM can't see or stop its process
	// (it was started with administrator rights): stopping/switching won't work.
	Unmanaged bool `json:"unmanaged,omitempty"`
	// StartError: why the last start failed (cleared by the next start).
	StartError string `json:"start_error,omitempty"`
	// UserStopped: the user pressed Stop; don't start it on their behalf.
	UserStopped bool               `json:"user_stopped"`
	Version     string             `json:"version"`
	Targets     []integrate.Target `json:"targets"`
}

// bus fans job events out to subscribers (the UI's event stream).
type bus struct {
	subMu sync.Mutex
	subs  map[chan Event]struct{}
	hist  []Event // events of the running job, replayed to new subscribers
}

func (b *bus) Subscribe() (chan Event, func()) {
	ch := make(chan Event, 256)
	b.subMu.Lock()
	if b.subs == nil {
		b.subs = map[chan Event]struct{}{}
	}
	for _, e := range b.hist {
		ch <- e
	}
	b.subs[ch] = struct{}{}
	b.subMu.Unlock()
	return ch, func() {
		b.subMu.Lock()
		delete(b.subs, ch)
		b.subMu.Unlock()
	}
}

func (b *bus) emit(e Event) {
	b.subMu.Lock()
	defer b.subMu.Unlock()
	// Progress ticks are not kept in history; only the latest matters.
	if e.State != "progress" {
		b.hist = append(b.hist, e)
	}
	for ch := range b.subs {
		select {
		case ch <- e:
		default: // slow subscriber: drop rather than block the job
		}
	}
	// A finished job isn't replayed: a client reconnecting later must not
	// receive an old "done" (it would act on a job that's long over).
	if e.Step == "done" {
		b.hist = nil
	}
}

func (b *bus) resetHistory() {
	b.subMu.Lock()
	b.hist = nil
	b.subMu.Unlock()
}

type App struct {
	bus
	mu     sync.Mutex
	cat    *catalog.Catalog
	last   *ScanResult
	busy   string
	cancel context.CancelFunc // cancels the running job

	// The last hardware scan, reused for a few seconds so switching between
	// Balanced / Smartest / Fastest is instant.
	hw      *scan.Hardware
	prereqs []prereq.Check
	hwAt    time.Time

	warming             bool // Start is reading the model before launching the server
	warmDone, warmTotal uint64
	startErr            string            // why the last Start failed, shown until the next one
	appErrs             map[string]string // by app id: why EZLM couldn't update its settings
}

// isEZLMDir guards deletes: only a folder EZLM itself created (one with
// "ezlm" in its path) is ever removed wholesale.
func isEZLMDir(dir string) bool {
	if dir == "" {
		return false
	}
	clean := filepath.Clean(dir)
	if filepath.Dir(clean) == clean { // a drive or filesystem root
		return false
	}
	return strings.Contains(strings.ToLower(clean), "ezlm")
}

// ErrCancelled marks a job the user stopped.
var ErrCancelled = errors.New("cancelled")

// Cancel stops the running job (downloads stop at once; finished parts are
// kept so the next attempt resumes).
func (a *App) Cancel() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel == nil {
		return errors.New("nothing is running")
	}
	a.cancel()
	return nil
}

func New() *App { return &App{} }

// Kill quits a process the scan listed (only the user's own processes).
func (a *App) Kill(ctx context.Context, pid int32) error {
	a.mu.Lock()
	a.hwAt = time.Time{}
	a.mu.Unlock()
	return scan.Kill(ctx, pid)
}

// hardware scans the machine, or reuses a scan from the last few seconds.
func (a *App) hardware(ctx context.Context) (*scan.Hardware, []prereq.Check) {
	a.mu.Lock()
	if a.hw != nil && time.Since(a.hwAt) < 20*time.Second {
		hw, pr := *a.hw, a.prereqs
		a.mu.Unlock()
		return &hw, pr
	}
	a.mu.Unlock()
	hw := scan.Scan(ctx)
	reclaimOwnServer(ctx, hw)
	pr := prereq.Run(ctx, hw)
	a.mu.Lock()
	cp := *hw
	a.hw, a.prereqs, a.hwAt = &cp, pr, time.Now()
	a.mu.Unlock()
	return hw, pr
}

// Apps lists programs using a lot of memory, for Model mode.
// At most 8: a short walk, not a task manager.
func (a *App) Apps(ctx context.Context) []scan.App {
	apps := scan.MemoryApps(ctx, 250<<20)
	if len(apps) > 8 {
		apps = apps[:8]
	}
	return apps
}

// CloseApp closes one of them (gently unless force), after the user said yes.
func (a *App) CloseApp(ctx context.Context, id string, force bool) (scan.CloseResult, error) {
	r, err := scan.CloseApp(ctx, id, force)
	a.mu.Lock()
	a.hwAt = time.Time{} // memory changed: plans need a fresh look
	a.mu.Unlock()
	return r, err
}

// Scan inspects the machine and computes plans for a preference
// ("balanced" by default, "smartest", "fastest").
func (a *App) Scan(ctx context.Context, pref string) *ScanResult {
	pf := fit.Pref(pref)
	if pf != fit.Smartest && pf != fit.Fastest {
		pf = fit.Balanced
	}
	hw, prereqs := a.hardware(ctx)
	cat, src := catalog.Load(ctx)

	res := &ScanResult{HW: hw, CatalogSource: src, Prereqs: prereqs, Pref: string(pf)}
	res.Plans = fit.PlansFor(hw, cat, fit.ComputeBudgets(hw, hw.RAMAvail), pf)
	if p, ok := fit.Recommend(res.Plans); ok {
		res.Recommended = &p
	}
	// What could the user run with other apps closed? Assume the OS keeps ~15%.
	// Only meaningful where RAM is the limit (unified memory or CPU-only).
	ideal := uint64(float64(hw.RAMTotal) * 0.85)
	if (hw.Unified || hw.Backend == scan.CPU) && ideal > hw.RAMAvail+2*scan.GB {
		up := fit.PlansFor(hw, cat, fit.ComputeBudgets(hw, ideal), pf)
		if p, ok := fit.Recommend(up); ok && (res.Recommended == nil || p.Model.Score > res.Recommended.Model.Score) {
			res.Unlock = &p
			res.UnlockFreeGB = float64(ideal-hw.RAMAvail) / float64(scan.GB)
		}
	}
	// Discrete GPU busy with other programs? Show what freeing it would unlock.
	if !hw.Unified && len(hw.GPUs) > 0 && res.Unlock == nil {
		var total uint64
		for _, g := range hw.GPUs {
			total += g.MemTotal
		}
		ideal := total - total/20 // the desktop itself keeps a little
		if ideal > hw.GPUUsable+2*scan.GB {
			hw2 := *hw
			hw2.GPUUsable = ideal
			up := fit.PlansFor(&hw2, cat, fit.ComputeBudgets(&hw2, hw.RAMAvail), pf)
			if p, ok := fit.Recommend(up); ok && (res.Recommended == nil || p.Model.Score > res.Recommended.Model.Score || p.TokPerSec > 1.5*res.Recommended.TokPerSec) {
				res.Unlock = &p
				res.UnlockFreeGB = float64(ideal-hw.GPUUsable) / float64(scan.GB)
				res.UnlockGPU = true
			}
		}
	}
	preferInstalled(res, hw, cat, pf)
	runningPlan(res, hw, cat, pf)
	markDownloaded(res, hw)
	a.mu.Lock()
	a.cat, a.last = cat, res
	a.mu.Unlock()
	return res
}

// modelDirs is every folder EZLM keeps (or has kept) models in: the default,
// the chosen download folder, the running setup's, and any used before.
func modelDirs(hw *scan.Hardware) []string {
	var out []string
	seen := map[string]bool{}
	add := func(d string) {
		if d == "" {
			return
		}
		k := strings.ToLower(filepath.Clean(d))
		if !seen[k] {
			seen[k] = true
			out = append(out, filepath.Clean(d))
		}
	}
	add(paths.Models())
	add(filepath.Join(paths.Home(), "models"))
	if hw != nil {
		add(hw.ModelsDir)
	}
	if cur, err := serve.Load(); err == nil {
		add(cur.ModelsDir)
	}
	st := paths.LoadSettings()
	add(st.ModelsDir)
	for _, d := range st.KnownDirs {
		add(d)
	}
	return out
}

// preferInstalled uses a file that's already downloaded when it runs nearly
// as well as the best file for this machine: offering a 2 GB download for a
// model that's already here (in a slightly different size) makes no sense.
func preferInstalled(res *ScanResult, hw *scan.Hardware, cat *catalog.Catalog, pf fit.Pref) {
	dirs := modelDirs(hw)
	have := func(m catalog.Model, q catalog.Quant) bool {
		for _, d := range dirs {
			if d != "" && complete(filepath.Join(d, m.ID, filepath.Base(q.Files[0])), q) {
				return true
			}
		}
		return false
	}
	b := fit.ComputeBudgets(hw, hw.RAMAvail)
	for i, best := range res.Plans {
		if have(best.Model, best.Quant) {
			continue
		}
		m, ok := cat.Find(best.Model.ID)
		if !ok {
			continue
		}
		for _, q := range m.Quants {
			if q.Name == best.Quant.Name || !have(m, q) {
				continue
			}
			if p, ok := fit.PlanForQuant(hw, m, q, b, pf); ok && fit.UtilityFor(p, pf) >= 0.85*fit.UtilityFor(best, pf) {
				res.Plans[i] = p
				if res.Recommended != nil && res.Recommended.Model.ID == m.ID {
					r := p
					res.Recommended = &r
				}
				break
			}
		}
	}
}

// runningPlan shows the model that's running as it actually runs (its file
// and setup), not as a different file of the same model that would need a
// download: "Running" next to "Needs space" was confusing.
func runningPlan(res *ScanResult, hw *scan.Hardware, cat *catalog.Catalog, pf fit.Pref) {
	cfg, err := serve.Load()
	if err != nil {
		return
	}
	m, ok := cat.Find(cfg.ModelID)
	if !ok {
		return
	}
	var q catalog.Quant
	for _, x := range m.Quants {
		if x.Name == cfg.Quant {
			q = x
		}
	}
	if q.Name == "" {
		return
	}
	b := fit.ComputeBudgets(hw, hw.RAMAvail)
	b.MinCtx = min(cfg.Ctx, 65536)
	p, ok := fit.PlanForQuant(hw, m, q, b, pf)
	if !ok {
		return
	}
	for i := range res.Plans {
		if res.Plans[i].Model.ID == cfg.ModelID {
			res.Plans[i] = p
		}
	}
	if res.Recommended != nil && res.Recommended.Model.ID == cfg.ModelID {
		r := p
		res.Recommended = &r
	}
}

// markDownloaded flags plans whose model files are already on disk: they need
// no download and no free space, so the UI mustn't warn about space for them.
func markDownloaded(res *ScanResult, hw *scan.Hardware) {
	dirs := modelDirs(hw)
	mark := func(p *fit.Plan) {
		if p == nil {
			return
		}
		for _, d := range dirs {
			if d != "" && complete(filepath.Join(d, p.Model.ID, filepath.Base(p.Quant.Files[0])), p.Quant) {
				p.Downloaded, p.DiskOK, p.Download = true, true, 0
				return
			}
		}
		// A cancelled download resumes: count what's already here.
		var have uint64
		for _, d := range dirs {
			if d == "" || have > 0 {
				continue
			}
			for _, f := range p.Quant.Files {
				name := filepath.Join(d, p.Model.ID, filepath.Base(f))
				for _, c := range []string{name, name + ".part"} {
					if st, err := os.Stat(c); err == nil {
						have += uint64(st.Size())
					}
				}
			}
		}
		if have > 0 && have < p.Download {
			p.Partial = have
			p.Download -= have
			p.DiskOK = p.Download+2*scan.GB < hw.DiskFree
		}
	}
	for i := range res.Plans {
		mark(&res.Plans[i])
	}
	mark(res.Recommended)
	mark(res.Unlock)
}

// reclaimOwnServer counts memory held by EZLM's own llama-server as free,
// since deploying a new model replaces it, and hides it from the
// "what's using your GPU" lists.
func reclaimOwnServer(ctx context.Context, hw *scan.Hardware) {
	pids := serve.OwnPIDs(ctx)
	if len(pids) == 0 {
		return
	}
	own := func(pid int32) bool {
		for _, p := range pids {
			if p == pid {
				return true
			}
		}
		return false
	}
	filter := func(ps []scan.Proc, reclaim func(scan.Proc)) []scan.Proc {
		var out []scan.Proc
		for _, p := range ps {
			if own(p.PID) {
				reclaim(p)
				continue
			}
			out = append(out, p)
		}
		return out
	}
	hw.GPUProcs = filter(hw.GPUProcs, func(p scan.Proc) { hw.GPUUsable += p.Mem })
	hw.TopMem = filter(hw.TopMem, func(scan.Proc) {})
	if rss := serve.OwnRSS(ctx, pids); rss > 0 {
		hw.RAMAvail = min(hw.RAMTotal, hw.RAMAvail+rss)
	}
	hw.Notes = append(hw.Notes, "Your current EZLM model is running; its memory is counted as free because a new deploy replaces it.")
}

func (a *App) Status(ctx context.Context) Status {
	a.mu.Lock()
	busy := a.busy
	a.mu.Unlock()
	s := Status{Busy: busy, Version: version.Version}
	cfg, err := serve.Load()
	if err != nil {
		s.Targets = integrate.Targets(nil)
		return s
	}
	ep := endpointFor(cfg)
	s.Targets = a.withAppErrs(integrate.Targets(&ep))
	s.Deployed, s.Config = true, cfg
	s.Healthy = serve.Healthy(ctx, cfg)
	own := len(serve.OwnPIDs(ctx)) > 0
	s.Starting = !s.Healthy && (a.isWarming() || own)
	s.Unmanaged = s.Healthy && !own
	_, ferr := os.Stat(serve.StoppedFlag())
	s.UserStopped = ferr == nil
	a.mu.Lock()
	if !s.Healthy {
		s.StartError = a.startErr
	}
	a.mu.Unlock()
	if s.Healthy && busy == "" {
		s.Tune = a.tune(ctx, cfg)
		if cfg.Ctx < 64000 {
			s.Hermes64 = a.ctxOption(ctx, cfg, 65536)
		}
	}
	s.LocalURL, s.LANURL = cfg.LocalURL(), cfg.LANURL()
	return s
}

// CtxOption is a way to run the current model with a bigger context.
type CtxOption struct {
	Fits      bool    `json:"fits"`
	TokPerSec float64 `json:"tok_per_sec"`
	Now       float64 `json:"now_tok_per_sec"`
}

// Tune describes a better way to run the model that's already running, e.g.
// after an EZLM update improved the recipe, or memory was freed up.
type Tune struct {
	Why   string `json:"why"`
	Quant string `json:"quant"`
}

// tune compares the running setup with what EZLM would pick today for the
// same model file. Small differences are ignored so it never nags.
func (a *App) tune(ctx context.Context, cfg *serve.Config) *Tune {
	if cfg.CPUFallback != "" {
		return nil // the card couldn't load this model: no "more fits on your graphics card" nudges
	}
	cat, _ := catalog.Load(ctx)
	m, ok := cat.Find(cfg.ModelID)
	if !ok {
		return nil
	}
	var q catalog.Quant
	for _, x := range m.Quants {
		if x.Name == cfg.Quant {
			q = x
		}
	}
	if q.Name == "" {
		return nil
	}
	hw, _ := a.hardware(ctx)
	b := fit.ComputeBudgets(hw, hw.RAMAvail)
	b.MinCtx = cfg.Ctx // never suggest less room than it has (Hermes needs 64K)
	p, ok := fit.PlanForQuant(hw, m, q, b, fit.Balanced)
	if !ok {
		return nil
	}
	switch {
	case p.NCPUMoE >= cfg.NCPUMoE+2:
		return &Tune{Quant: q.Name, Why: "Leaves room on your graphics card for reading long prompts, so coding agents get answers much sooner."}
	case cfg.NCPUMoE > 0 && p.NCPUMoE <= cfg.NCPUMoE-3, p.NGL < 999 && p.NGL >= cfg.NGL+3, p.NGL == 999 && cfg.NGL < 999:
		return &Tune{Quant: q.Name, Why: "More of the model fits on your graphics card now, so replies come faster."}
	case p.Ctx > cfg.Ctx:
		return &Tune{Quant: q.Name, Why: fmt.Sprintf("Gives it a %dK context instead of %dK, so it can work with longer conversations and files.", p.Ctx/1024, cfg.Ctx/1024)}
	}
	return nil
}

// ctxOption estimates the speed with a bigger context (cached hardware scan).
func (a *App) ctxOption(ctx context.Context, cfg *serve.Config, want int) *CtxOption {
	cat, _ := catalog.Load(ctx)
	m, ok := cat.Find(cfg.ModelID)
	if !ok {
		return nil
	}
	var q catalog.Quant
	for _, x := range m.Quants {
		if x.Name == cfg.Quant {
			q = x
		}
	}
	hw, _ := a.hardware(ctx)
	b := fit.ComputeBudgets(hw, hw.RAMAvail)
	now, ok := fit.PlanForQuant(hw, m, q, b, fit.Balanced)
	if !ok {
		return nil
	}
	b.MinCtx = want
	big, ok := fit.PlanForQuant(hw, m, q, b, fit.Balanced)
	return &CtxOption{Fits: ok, TokPerSec: big.TokPerSec, Now: now.TokPerSec}
}

var ErrBusy = errors.New("EZLM is already working on something")

// start runs fn as a background job unless one is already running.
func (a *App) start(name string, fn func(ctx context.Context, step func(step, state, msg string)) error) error {
	a.mu.Lock()
	if a.busy != "" {
		a.mu.Unlock()
		return ErrBusy
	}
	a.busy = name
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.mu.Unlock()

	a.resetHistory()

	go func() {
		// A download can take an hour: don't let the computer doze off mid-way.
		release := awake.Hold("EZLM is setting up your AI model")
		defer release()
		step := func(step, state, msg string) { a.emit(Event{Job: name, Step: step, State: state, Msg: msg}) }
		err := fn(ctx, step)
		cancelled := ctx.Err() != nil
		cancel()
		a.mu.Lock()
		a.busy, a.cancel = "", nil
		a.hwAt = time.Time{} // memory use changed: rescan next time
		a.mu.Unlock()
		if cancelled {
			a.emit(Event{Job: name, Step: "done", State: "cancelled", Msg: "Cancelled"})
		} else if err != nil {
			a.emit(Event{Job: name, Step: "done", State: "fail", Msg: err.Error()})
		} else {
			a.emit(Event{Job: name, Step: "done", State: "ok", Data: a.Status(context.Background())})
		}
	}()
	return nil
}

func (a *App) progress(job, step string) engine.Progress {
	return func(done, total uint64, bps float64) {
		a.emit(Event{Job: job, Step: step, State: "progress", Done: done, Total: total, BPS: bps})
	}
}

// Deploy installs and starts the given model (the recommended one if id is "").
func (a *App) Deploy(modelID string) error {
	a.mu.Lock()
	last := a.last
	a.mu.Unlock()
	if last == nil {
		return errors.New("scan the machine first")
	}
	var plan *fit.Plan
	for i := range last.Plans {
		if last.Plans[i].Model.ID == modelID || (modelID == "" && last.Recommended != nil && last.Plans[i].Model.ID == last.Recommended.Model.ID) {
			plan = &last.Plans[i]
			break
		}
	}
	if plan == nil {
		return fmt.Errorf("model %q doesn't fit this machine", modelID)
	}
	return a.deployPlan(last.HW, *plan)
}

// deployPlan installs and starts a specific plan (download, server, verify),
// keeping the URL and keys and restoring the previous model on failure.
func (a *App) deployPlan(hw *scan.Hardware, p fit.Plan) error {
	return a.start("deploy", func(ctx context.Context, step func(string, string, string)) (err error) {
		if err := paths.Ensure(); err != nil {
			return err
		}
		for _, c := range prereq.Run(ctx, hw) {
			switch {
			case c.ID == "vcredist" && c.Status == prereq.Fix:
				step("engine", "run", "Installing the Microsoft Visual C++ runtime the engine needs")
				dl := func(url, dst string) error { return engine.Download(ctx, url, dst, 0, nil) }
				if err := prereq.FixVCRedist(ctx, dl, paths.Bin()); err != nil {
					step("engine", "fail", "Couldn't install the Visual C++ runtime: "+err.Error())
					return err
				}
			case c.Status == prereq.Fail:
				step("engine", "fail", c.Name+": "+c.Action)
				return errors.New(c.Action)
			}
		}
		step("engine", "run", "Getting the AI engine (llama.cpp) for your "+string(hw.Backend)+" hardware")
		inst, err := engine.Install(ctx, hw, a.progress("deploy", "engine"))
		if err != nil {
			step("engine", "fail", err.Error())
			return err
		}
		step("engine", "ok", "Engine ready ("+inst.Tag+")")

		paths.SetModels(hw.ModelsDir)
		paths.RememberModelsDir(hw.ModelsDir)
		if st := paths.LoadSettings(); st.ModelsDir == "" && st.AutoDir == "" {
			st.AutoDir = hw.ModelsDir // from now on, downloads stay on this drive
			paths.SaveSettings(st)
		}
		target := engine.ModelPath(p.Model, p.Quant)
		targetDraft := ""
		if p.Model.Draft != nil {
			targetDraft = filepath.Join(paths.Models(), p.Model.ID, filepath.Base(p.Model.Draft.File))
		}
		// Already downloaded into any folder EZLM has used? Use it there:
		// never download (or make room for) a model that's already here.
		found := ""
		for _, d := range modelDirs(hw) {
			if cand := filepath.Join(d, p.Model.ID, filepath.Base(p.Quant.Files[0])); complete(cand, p.Quant) {
				found, target = cand, cand
				if p.Model.Draft != nil {
					if dp := filepath.Join(d, p.Model.ID, filepath.Base(p.Model.Draft.File)); fileExists(dp) {
						targetDraft = dp
					}
				}
				break
			}
		}
		if found != "" {
			// Nothing to download, so nothing to make room for.
			p.Download, p.DiskOK = 0, true
			step("model", "run", fmt.Sprintf("Using %s already on this computer (no download)", p.Model.Name))
		} else {
			step("model", "run", fmt.Sprintf("Downloading %s, %s (%.1f GB)", p.Model.Name, p.Quant.Name, float64(p.Download)/float64(scan.GB)))
		}
		if !p.DiskOK {
			// Make room from EZLM's own superseded models first (never the one
			// running: it's what we restore if this install fails).
			if cur, err := serve.Load(); err == nil {
				if freed := pruneUnused(cur, p.Download+2*scan.GB-min(hw.DiskFree, p.Download+2*scan.GB), p.Model.ID, target, targetDraft); freed > 0 {
					step("model", "info", fmt.Sprintf("Freed %.0f GB by removing EZLM models you're no longer using", float64(freed)/float64(scan.GB)))
					if du, err := disk.Usage(paths.Models()); err == nil {
						hw.DiskFree = du.Free
						p.DiskOK = p.Download+2*scan.GB < du.Free
					}
				}
			}
		}
		replacedCurrent := false
		if !p.DiskOK {
			if cur, err := serve.Load(); err == nil && cur.ModelPath != "" {
				curBytes := modelBytes(cur.ModelPath)
				if hw.DiskFree+curBytes > p.Download+2*scan.GB {
					step("model", "info", fmt.Sprintf("Not enough room for both models: removing %s (%.0f GB) first. If the new one fails, EZLM will need to download it again.", cur.ModelName, float64(curBytes)/float64(scan.GB)))
					if err := serve.StopAndWait(cur); err != nil {
						step("model", "fail", err.Error())
						return err
					}
					removeModel(cur.ModelPath)
					replacedCurrent = true
					if du, err := disk.Usage(paths.Models()); err == nil {
						hw.DiskFree = du.Free
						p.DiskOK = p.Download+2*scan.GB < du.Free
					}
				}
			}
		}
		_ = replacedCurrent
		if !p.DiskOK {
			step("model", "info", fmt.Sprintf("Heads up: this needs about %.0f GB and %.0f GB is free. Make room now (empty the Trash, remove big files) or the download will pause when the disk fills.",
				float64(p.Download)/float64(scan.GB)+2, float64(hw.DiskFree)/float64(scan.GB)))
		}
		modelPath := found
		if modelPath == "" {
			modelPath, err = engine.DownloadModel(ctx, p.Model, p.Quant, a.progress("deploy", "model"))
		}
		if err != nil {
			if isDiskFull(err) {
				err = fmt.Errorf("the disk filled up during the download. Free about %.0f GB and press Set it up again: EZLM resumes where it stopped", float64(p.Download)/float64(scan.GB)+2)
			}
			step("model", "fail", err.Error())
			return err
		}
		draftPath := ""
		if p.Model.Draft != nil && p.Spec == p.Model.Draft.Type {
			if found != "" && fileExists(targetDraft) {
				draftPath = targetDraft
			} else if draftPath, err = engine.DownloadDraft(ctx, p.Model, a.progress("deploy", "model")); err != nil {
				step("model", "fail", err.Error())
				return err
			}
		}
		step("model", "ok", p.Model.Name+" ready")

		step("start", "run", "Starting your server")
		old, _ := serve.Load()
		failed := true // cleared once the new model answers
		cfg := &serve.Config{
			ModelID: p.Model.ID, ModelName: p.Model.Name, Quant: p.Quant.Name, ModelPath: modelPath,
			ModelsDir: hw.ModelsDir, Ctx: p.Ctx, NGL: p.NGL, NCPUMoE: p.NCPUMoE, Server: inst.Server, EngineTag: inst.Tag,
			ToolCalls: p.Model.ToolCalling, DeployedAt: time.Now(),
			DraftPath: draftPath, ExtraArgs: p.Args(), Recipe: p.Recipe(), GPUNeed: p.GPUNeed, RAMNeed: p.RAMNeed,
		}
		if old != nil {
			// Keep the user's URL and key stable across model swaps.
			cfg.Key, cfg.Port, cfg.LAN = old.Key, old.Port, old.LAN
			// Make sure the old server is really gone: otherwise the new one
			// would end up on another port with two servers running.
			if err := serve.StopAndWait(old); err != nil {
				step("start", "fail", err.Error())
				return err
			}
			// If anything from here on fails or is cancelled, put the previous
			// model back so the user is never left without a working server.
			defer func() {
				if ctx.Err() != nil || failed {
					a.emit(Event{Job: "deploy", Step: "start", State: "info", Msg: "Restoring " + old.ModelName})
					old.Save()
					if rerr := serve.Install(old); rerr != nil && err != nil {
						err = fmt.Errorf("%v. Putting %s back also failed (%v); press Start server on Overview", err, old.ModelName, rerr)
					}
				}
			}()
		}
		if old != nil {
			cfg.Keys = old.Keys
		}
		if len(cfg.Keys) == 0 {
			cfg.Keys = []serve.APIKey{{ID: "k1", Name: "Default", Key: keys.New(), Created: time.Now()}}
		}
		if cfg.Port == 0 || !serve.PortFree(cfg.Port) {
			cfg.Port = serve.FreePort(max(cfg.Port, 8080))
		}
		if err := cfg.Save(); err != nil {
			return err
		}
		integrate.NotePort(cfg.Port)
		if cfg.NCPUMoE > 0 || cfg.NGL < 999 {
			step("start", "info", "Reading the model into memory first, so your first questions are fast")
			engine.Prewarm(ctx, engine.SplitFiles(cfg.ModelPath), a.progress("deploy", "load"))
		}
		if err := serve.Install(cfg); err != nil {
			step("start", "fail", err.Error())
			return err
		}
		step("start", "ok", fmt.Sprintf("Server registered on port %d", cfg.Port))

		step("load", "run", "Loading the model into memory")
		loading := func(el time.Duration) {
			a.emit(Event{Job: "deploy", Step: "load", State: "progress", Msg: fmt.Sprintf("%ds", int(el.Seconds()))})
		}
		err = serve.WaitHealthy(ctx, cfg, 10*time.Minute, loading)
		loaded := "Model loaded"
		if cpu, ok := cpuInstead(cfg, hw, p); err != nil && ctx.Err() == nil && ok && driverFailure(err.Error()) {
			why := firstLine(err.Error())
			step("load", "info", fmt.Sprintf("Your graphics card couldn't load it (%s), so EZLM is trying the processor instead.", why))
			serve.Stop()
			// The processor-only build loads no graphics driver at all, so a
			// broken one can't get in the way (if it can't be fetched, the same
			// build with no graphics device is the next best thing).
			onlyCPU := *hw
			onlyCPU.Backend = scan.CPU
			if inst, ierr := engine.Install(ctx, &onlyCPU, a.progress("deploy", "load")); ierr == nil {
				cpu.Server = inst.Server
			}
			cpu.CPUFallback = why
			cfg = cpu
			loaded = "Model loaded on the processor (the graphics card couldn't load it). A newer graphics driver may fix this; then set the model up again in Models"
			if err = cfg.Save(); err == nil {
				if err = serve.Install(cfg); err == nil {
					err = serve.WaitHealthy(ctx, cfg, 10*time.Minute, loading)
				}
			}
		}
		if err != nil {
			serve.Stop()
			step("load", "fail", err.Error())
			return err
		}
		step("load", "ok", loaded)

		step("test", "run", "Asking your model a question")
		// Models that think before answering can take a minute: show it ticking.
		asked := make(chan struct{})
		go func() {
			tick := time.NewTicker(time.Second)
			defer tick.Stop()
			for start := time.Now(); ; {
				select {
				case <-asked:
					return
				case <-tick.C:
					a.emit(Event{Job: "deploy", Step: "test", State: "progress", Msg: fmt.Sprintf("%ds", int(time.Since(start).Seconds()))})
				}
			}
		}()
		res, err := serve.Test(ctx, cfg, "In one short sentence, say hello and tell me what you are.")
		close(asked)
		if err != nil {
			step("test", "fail", err.Error())
			return err
		}
		a.emit(Event{Job: "deploy", Step: "test", State: "ok", Msg: res.Reply, Data: res})
		failed = false
		// What the model's template accepts for thinking, for the writers.
		a.refreshProps(ctx, cfg)
		// Point connected apps at the new model (id and context size). An
		// app with a context floor (Hermes needs 64K) gets the room only
		// when it barely costs speed; the model is never made much slower
		// for everyone behind the user's back. Otherwise that app is left
		// as it is and shows needs_64k.
		ep := endpointFor(cfg)
		for _, t := range integrate.Targets(&ep) {
			if !t.Linked || t.MinCtx == 0 || cfg.Ctx >= t.MinCtx {
				continue
			}
			if cfg.CPUFallback != "" {
				continue // on the processor a bigger context is never cheap
			}
			if hp, ok := a.contextPlan(ctx, cfg, ctxFor(t.MinCtx)); ok && hp.Mode == p.Mode && hp.TokPerSec >= 0.75*p.TokPerSec {
				step("test", "info", fmt.Sprintf("%s needs a %dK context: giving the model more room", t.Name, t.MinCtx/1000))
				if err := a.applyPlan(ctx, cfg, hp); err != nil {
					step("test", "info", err.Error())
				}
				continue
			}
			step("test", "info", fmt.Sprintf("Kept a %dK context so the model stays fast.", cfg.Ctx/1024))
		}
		a.rewriteLinked(cfg, func(msg string) { step("test", "info", msg) })
		now := time.Now()
		os.Chtimes(cfg.ModelPath, now, now) // last used: the library prunes least-recently-used first
		return nil
	})
}

// modelBytes is the size of a model, including every part of a split GGUF.
func modelBytes(first string) uint64 {
	var n uint64
	filepath.WalkDir(filepath.Dir(first), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && (filepath.Clean(p) == filepath.Clean(first) || sameSplitSet(p, first)) {
			if info, err := d.Info(); err == nil {
				n += uint64(info.Size())
			}
		}
		return nil
	})
	return n
}

func removeModel(first string) {
	filepath.WalkDir(filepath.Dir(first), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && (filepath.Clean(p) == filepath.Clean(first) || sameSplitSet(p, first)) {
			os.Remove(p)
		}
		return nil
	})
}

func isDiskFull(err error) bool {
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "no space left") || strings.Contains(m, "not enough space") || strings.Contains(m, "disk full")
}

// protected reports whether p is one of the protected files or another shard
// of one.
func protected(p string, protect []string) bool {
	for _, q := range protect {
		if q != "" && sameSplitSet(p, q) {
			return true
		}
	}
	return false
}

// pruneUnused deletes EZLM model files that cfg doesn't use, least recently
// used first, until need bytes are freed (0 = all). Returns bytes freed.
// Partial downloads (.part) are kept: they let a retry resume.
func pruneUnused(cfg *serve.Config, need uint64, keepModel string, protect ...string) uint64 {
	keep := map[string]bool{filepath.Clean(cfg.ModelPath): true, filepath.Clean(cfg.DraftPath): true}
	for _, p := range protect {
		keep[filepath.Clean(p)] = true
	}
	type file struct {
		path string
		size uint64
		used time.Time
	}
	var files []file
	for _, root := range modelDirs(nil) {
		if root == "" {
			continue
		}
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".gguf") || keep[filepath.Clean(p)] || sameSplitSet(p, cfg.ModelPath) || protected(p, protect) ||
				(keepModel != "" && strings.EqualFold(filepath.Base(filepath.Dir(p)), keepModel)) {
				return nil
			}
			if info, err := d.Info(); err == nil {
				files = append(files, file{p, uint64(info.Size()), info.ModTime()})
			}
			return nil
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].used.Before(files[j].used) })
	var freed uint64
	for _, f := range files {
		if need > 0 && freed >= need {
			break
		}
		if os.Remove(f.path) == nil {
			freed += f.size
		}
	}
	return freed
}

// sameSplitSet reports whether p is another part of a split GGUF whose first
// part is first (…-00002-of-00003.gguf belongs to …-00001-of-00003.gguf).
func sameSplitSet(p, first string) bool {
	i := strings.LastIndex(first, "-00001-of-")
	return i > 0 && strings.HasPrefix(p, first[:i]) && strings.HasSuffix(p, first[i+len("-00001"):])
}

// SetLAN exposes or hides the server on the local network.
func (a *App) SetLAN(on bool) error {
	cfg, err := serve.Load()
	if err != nil {
		return errors.New("nothing is deployed yet")
	}
	return a.start("lan", func(ctx context.Context, step func(string, string, string)) error {
		cfg.LAN = on
		if err := cfg.Save(); err != nil {
			return err
		}
		step("lan", "run", "Restarting the server")
		if err := serve.Install(cfg); err != nil {
			return err
		}
		if err := serve.WaitHealthy(ctx, cfg, 10*time.Minute, nil); err != nil {
			return err
		}
		step("lan", "ok", map[bool]string{true: "Visible on your network", false: "Only this computer can connect"}[on])
		return nil
	})
}

// Integrate wires the deployed model into the chosen apps. The server is
// started first if it isn't running; an app with a context floor above the
// model's (Hermes needs 64K) gets the room by restarting the server with a
// bigger context, and the other connected apps follow.
func (a *App) Integrate(ids []string, makeDefault bool) error {
	cfg, err := serve.Load()
	if err != nil {
		return errors.New("nothing is deployed yet")
	}
	if len(ids) == 0 {
		return errors.New("pick an app to connect")
	}
	return a.start("integrate", func(ctx context.Context, step func(string, string, string)) error {
		first := "integrate:" + ids[0]
		if err := a.serverGate(ctx, cfg, func(msg string) { step(first, "run", msg) }); err != nil {
			step(first, "fail", err.Error())
			return err
		}
		want, who := 0, ""
		for _, id := range ids {
			if t, ok := integrate.Lookup(id); ok {
				if inf := t.Info(); inf.MinCtx > cfg.Ctx && ctxFor(inf.MinCtx) > want {
					want, who = ctxFor(inf.MinCtx), id
				}
			}
		}
		if want > 0 {
			t, _ := integrate.Lookup(who)
			inf := t.Info()
			s := "integrate:" + who
			step(s, "run", fmt.Sprintf("%s needs a %dK context window: restarting your server with more room", inf.Name, inf.MinCtx/1000))
			if err := a.raiseContext(ctx, cfg, want, inf.Name); err != nil {
				step(s, "fail", err.Error())
				return err
			}
			a.refreshProps(ctx, cfg)
			a.rewriteLinked(cfg, func(msg string) { step(s, "info", msg) })
		}
		ep := endpointFor(cfg)
		var failed []string
		for _, id := range ids {
			s := "integrate:" + id
			name := id
			if t, ok := integrate.Lookup(id); ok {
				name = t.Info().Name
			}
			step(s, "run", "Setting up "+name)
			err := integrate.Setup(ctx, id, ep, makeDefault, func(msg string) { step(s, "info", msg) })
			if err != nil {
				step(s, "fail", err.Error())
				failed = append(failed, name)
				continue
			}
			a.setAppErr(id, "")
			step(s, "ok", "Ready")
		}
		if len(failed) > 0 {
			return fmt.Errorf("EZLM couldn't finish connecting %s. The reason is shown next to each app", strings.Join(failed, " and "))
		}
		return nil
	})
}

// InstalledModel is a model file already on this computer.
type InstalledModel struct {
	ModelID string    `json:"model_id"`
	Name    string    `json:"name"`
	Vendor  string    `json:"vendor"`
	Country string    `json:"country"`
	Quant   string    `json:"quant"`
	Bytes   uint64    `json:"bytes"`
	Running bool      `json:"running"`
	Plan    *fit.Plan `json:"plan,omitempty"` // how it would run here now (nil if it can't)
}

// Installed lists catalog models whose files are fully downloaded.
func (a *App) Installed(ctx context.Context, pref string) []InstalledModel {
	cat, _ := catalog.Load(ctx)
	hw := scan.Scan(ctx)
	reclaimOwnServer(ctx, hw)
	b := fit.ComputeBudgets(hw, hw.RAMAvail)
	cur, _ := serve.Load()
	dirs := modelDirs(hw)
	var out []InstalledModel
	seen := map[string]bool{}
	for _, m := range cat.Models {
		for _, q := range m.Quants {
			for _, d := range dirs {
				first := filepath.Join(d, m.ID, filepath.Base(q.Files[0]))
				if seen[first] || !complete(first, q) {
					continue
				}
				seen[first] = true
				im := InstalledModel{ModelID: m.ID, Name: m.Name, Vendor: m.Vendor, Country: m.Country, Quant: q.Name, Bytes: q.Bytes,
					Running: cur != nil && filepath.Clean(cur.ModelPath) == filepath.Clean(first)}
				if p, ok := fit.PlanForQuant(hw, m, q, b, fit.Pref(pref)); ok {
					im.Plan = &p
				}
				out = append(out, im)
			}
		}
	}
	return out
}

// complete reports whether every part of a quant is fully downloaded.
func complete(first string, q catalog.Quant) bool {
	var total uint64
	for _, f := range engine.SplitFiles(first) {
		st, err := os.Stat(f)
		if err != nil {
			return false
		}
		total += uint64(st.Size())
	}
	return len(q.Files) > 1 || total == q.Bytes || q.Bytes == 0
}

// Use switches to a model, by default reusing a downloaded file. quant may be
// "" to take the best file for this machine (downloading if needed).
func (a *App) Use(ctx context.Context, modelID, quant, pref string) error {
	cat, _ := catalog.Load(ctx)
	m, ok := cat.Find(modelID)
	if !ok {
		return fmt.Errorf("unknown model %q", modelID)
	}
	hw := scan.Scan(ctx)
	reclaimOwnServer(ctx, hw)
	b := fit.ComputeBudgets(hw, hw.RAMAvail)
	if cur, err := serve.Load(); err == nil && cur.ModelID == modelID {
		b.MinCtx = cur.Ctx // re-tuning the running model: keep at least its context
	}
	if quant == "" {
		for _, im := range a.Installed(ctx, pref) {
			if im.ModelID == modelID && im.Plan != nil {
				quant = im.Quant
				break
			}
		}
	}
	if quant == "" {
		p, ok := fit.Recommend(fit.PlansFor(hw, &catalog.Catalog{Models: []catalog.Model{m}}, b, fit.Pref(pref)))
		if !ok {
			return fmt.Errorf("%s doesn't fit this machine right now", m.Name)
		}
		return a.deployPlan(hw, p)
	}
	for _, q := range m.Quants {
		if q.Name == quant {
			p, ok := fit.PlanForQuant(hw, m, q, b, fit.Pref(pref))
			if !ok {
				return fmt.Errorf("%s (%s) doesn't fit in the memory that's free right now", m.Name, quant)
			}
			return a.deployPlan(hw, p)
		}
	}
	return fmt.Errorf("%s has no file %q", m.Name, quant)
}

// Remove deletes a downloaded model (never the one that's running).
func (a *App) Remove(ctx context.Context, modelID, quant string) error {
	for _, im := range a.Installed(ctx, "") {
		if im.ModelID != modelID || (quant != "" && im.Quant != quant) {
			continue
		}
		if im.Running {
			return errors.New("that's the model you're running; switch to another one first")
		}
		cat, _ := catalog.Load(ctx)
		m, _ := cat.Find(modelID)
		for _, q := range m.Quants {
			if q.Name != im.Quant {
				continue
			}
			for _, d := range modelDirs(nil) {
				first := filepath.Join(d, m.ID, filepath.Base(q.Files[0]))
				if _, err := os.Stat(first); err == nil {
					removeModel(first)
				}
				removeOrphanDraft(filepath.Join(d, m.ID), m)
				os.Remove(filepath.Join(d, m.ID)) // the folder, once empty
			}
		}
		return nil
	}
	return errors.New("that model isn't on this computer")
}

// removeOrphanDraft deletes a model's speed-up (draft) file once no other
// file of that model is left to use it.
func removeOrphanDraft(dir string, m catalog.Model) {
	if m.Draft == nil {
		return
	}
	draft := filepath.Base(m.Draft.File)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".gguf") && e.Name() != draft {
			return // another quant still uses it
		}
	}
	os.Remove(filepath.Join(dir, draft))
	os.Remove(dir) // only succeeds when empty
}

// AddKey creates a new named key and restarts the server so it's accepted.
func (a *App) AddKey(name string) error {
	return a.editKeys("Creating a new key", func(cfg *serve.Config) error {
		if name == "" {
			name = fmt.Sprintf("Key %d", len(cfg.Keys)+1)
		}
		cfg.Keys = append(cfg.Keys, serve.APIKey{ID: fmt.Sprintf("k%x", time.Now().UnixNano()&0xffffff), Name: name, Key: keys.New(), Created: time.Now()})
		return nil
	})
}

// RotateKey replaces a key's value. Apps EZLM connected are updated when the
// rotated key is the one they use, so nothing breaks.
func (a *App) RotateKey(id string) error {
	return a.editKeys("Rotating the key", func(cfg *serve.Config) error {
		for i := range cfg.Keys {
			if cfg.Keys[i].ID == id {
				cfg.Keys[i].Key, cfg.Keys[i].Created = keys.New(), time.Now()
				return nil
			}
		}
		return errors.New("that key no longer exists")
	})
}

// RevokeKey deletes a key. The last key can't be removed.
func (a *App) RevokeKey(id string) error {
	return a.editKeys("Revoking the key", func(cfg *serve.Config) error {
		if len(cfg.Keys) <= 1 {
			return errors.New("the server needs at least one key")
		}
		for i := range cfg.Keys {
			if cfg.Keys[i].ID == id {
				cfg.Keys = append(cfg.Keys[:i], cfg.Keys[i+1:]...)
				return nil
			}
		}
		return errors.New("that key no longer exists")
	})
}

// editKeys applies a key change, rewrites connected apps' configs if the
// primary key changed, and restarts the server.
func (a *App) editKeys(what string, edit func(*serve.Config) error) error {
	cfg, err := serve.Load()
	if err != nil {
		return errors.New("nothing is deployed yet")
	}
	before := cfg.Key
	if err := edit(cfg); err != nil {
		return err
	}
	return a.start("keys", func(ctx context.Context, step func(string, string, string)) error {
		if err := cfg.Save(); err != nil {
			return err
		}
		var updated []string
		if cfg.Key != before {
			step("keys", "info", "Updating connected apps")
			updated = a.rewriteLinked(cfg, func(msg string) { step("keys", "info", msg) })
		}
		step("keys", "run", what+": restarting the server")
		if err := installService(cfg); err != nil {
			return err
		}
		if err := serverWait(ctx, cfg, 10*time.Minute, nil); err != nil {
			return err
		}
		if note := restartNote(updated); note != "" {
			step("keys", "info", note)
		}
		step("keys", "ok", "Done")
		return nil
	})
}

// Test runs the model check suite as a job; each result is a "test" event.
func (a *App) Test() error {
	cfg, err := serve.Load()
	if err != nil {
		return errors.New("nothing is deployed yet")
	}
	return a.start("test", func(ctx context.Context, step func(string, string, string)) error {
		if !serve.Healthy(ctx, cfg) {
			// Never test whatever else answers on this port.
			step("test", "fail", "Your server isn't running. Press Start server, then test again.")
			return errors.New("your server isn't running")
		}
		t := bench.Target{BaseURL: cfg.LocalURL(), Key: cfg.Key, Model: cfg.ModelID}
		bench.Run(ctx, t, func(r bench.Result) {
			a.emit(Event{Job: "test", Step: "result", State: map[bool]string{true: "ok", false: "fail"}[r.Pass], Msg: r.Name, Data: r})
		})
		return nil
	})
}

// contextPlan is how the running model would be set up with at least want
// tokens of context (graphics/system memory split re-planned to fit).
func (a *App) contextPlan(ctx context.Context, cfg *serve.Config, want int) (fit.Plan, bool) {
	cat, _ := catalog.Load(ctx)
	m, ok := cat.Find(cfg.ModelID)
	if !ok {
		return fit.Plan{}, false
	}
	var q catalog.Quant
	for _, x := range m.Quants {
		if x.Name == cfg.Quant {
			q = x
		}
	}
	if q.Name == "" {
		return fit.Plan{}, false
	}
	hw := scan.Scan(ctx)
	reclaimOwnServer(ctx, hw)
	b := fit.ComputeBudgets(hw, hw.RAMAvail)
	b.MinCtx = want
	return fit.PlanForQuant(hw, m, q, b, fit.Balanced)
}

// raiseContext restarts the server with at least want tokens of context,
// re-planning how the model is split between graphics card and system memory
// so the bigger cache doesn't overflow the card (on Windows that doesn't fail,
// it just gets very slow).
func (a *App) raiseContext(ctx context.Context, cfg *serve.Config, want int, forApp string) error {
	p, ok := a.contextPlan(ctx, cfg, want)
	if !ok {
		return fmt.Errorf("%s can't have a %dK context on this machine; pick a smaller model in Models to use %s", cfg.ModelName, want/1024, forApp)
	}
	return a.applyPlan(ctx, cfg, p)
}

func (a *App) applyPlan(ctx context.Context, cfg *serve.Config, p fit.Plan) error {
	old := *cfg
	cfg.Ctx, cfg.NGL, cfg.NCPUMoE, cfg.ExtraArgs, cfg.Recipe = p.Ctx, p.NGL, p.NCPUMoE, p.Args(), p.Recipe()
	cfg.GPUNeed, cfg.RAMNeed = p.GPUNeed, p.RAMNeed
	if cfg.CPUFallback != "" { // the card couldn't load this model: stay on the processor
		cfg.NGL, cfg.NCPUMoE = 0, 0
		cfg.ExtraArgs = append(cfg.ExtraArgs, "--device", "none")
	}
	if p.Spec == "" {
		cfg.DraftPath = ""
	}
	if err := cfg.Save(); err == nil {
		if err = serve.Install(cfg); err == nil {
			if err = serve.WaitHealthy(ctx, cfg, 10*time.Minute, nil); err == nil {
				return nil
			}
		}
	}
	*cfg = old
	cfg.Save()
	serve.Install(cfg)
	serve.WaitHealthy(ctx, cfg, 10*time.Minute, nil)
	return fmt.Errorf("couldn't restart %s with a %dK context; your previous setup is back", cfg.ModelName, p.Ctx/1024)
}

// Stop stops the server and remembers it was the user's choice, so neither
// login nor opening the app starts it again until they press Start.
func (a *App) Stop() error {
	os.WriteFile(serve.StoppedFlag(), []byte("stopped by the user\n"), 0o600)
	return serve.Stop()
}

// Start starts the server in the background. Models split with system memory
// are read through first so the first prompts aren't slow; meanwhile Status
// and Live report it as starting.
func (a *App) Start() error {
	cfg, err := serve.Load()
	if err != nil {
		return errors.New("nothing is set up yet")
	}
	os.Remove(serve.StoppedFlag())
	if len(serve.OwnPIDs(context.Background())) > 0 {
		return nil
	}
	a.mu.Lock()
	if a.warming {
		a.mu.Unlock()
		return nil
	}
	a.warming, a.startErr = true, ""
	a.mu.Unlock()
	go func() {
		defer func() { a.mu.Lock(); a.warming, a.warmDone, a.warmTotal = false, 0, 0; a.mu.Unlock() }()
		err := a.StartSync(context.Background(), cfg, func(done, total uint64, _ float64) {
			a.mu.Lock()
			a.warmDone, a.warmTotal = done, total
			a.mu.Unlock()
		})
		if err != nil {
			a.mu.Lock()
			// The app's heading already says it couldn't start: keep the why.
			msg := strings.TrimPrefix(err.Error(), "the model server couldn't start: ")
			if msg != "" {
				msg = strings.ToUpper(msg[:1]) + msg[1:]
			}
			a.startErr = msg
			a.mu.Unlock()
		}
	}()
	return nil
}

// StartSync starts the server and waits until it's running (or has failed):
// the pre-read for split models, moving off a port another program took
// (connected apps follow), and a plain-language reason if it dies.
func (a *App) StartSync(ctx context.Context, cfg *serve.Config, warm engine.Progress) error {
	if len(serve.OwnPIDs(ctx)) > 0 {
		return nil
	}
	if !serve.PortFree(cfg.Port) {
		if serve.Healthy(ctx, cfg) {
			return nil // our own server already answers (e.g. one started as administrator)
		}
		// Something else (a dev tool, a proxy) holds our port: move, and point
		// connected apps at the new address.
		old := cfg.Port
		integrate.NotePort(old) // configs that still say the old port stay EZLM's
		cfg.Port = serve.FreePort(cfg.Port + 1)
		if err := cfg.Save(); err != nil {
			return err
		}
		integrate.NotePort(cfg.Port)
		msg := fmt.Sprintf("Port %d was taken by another program; your server moved to %d and connected apps were updated.", old, cfg.Port)
		if note := restartNote(a.rewriteApps(cfg)); note != "" {
			msg += " " + note
		}
		a.emit(Event{Job: "start", Step: "start", State: "info", Msg: msg})
	}
	if !fileExists(cfg.Server) {
		if err := a.restoreServer(ctx, cfg); err != nil {
			return err
		}
	}
	if cfg.NCPUMoE > 0 || cfg.NGL < 999 {
		engine.Prewarm(ctx, engine.SplitFiles(cfg.ModelPath), warm)
	}
	if err := serve.Start(); err != nil {
		return err
	}
	// It counts as started once the same process has stayed up for a bit
	// (big models take minutes to load). If it exits, or the service manager
	// restarts it (launchd and systemd do, so a crash can look like a running
	// server), or its log shows a fatal error, it failed: say why and stop it
	// so it doesn't crash in a loop.
	fail := func() error {
		reason := serverReason()
		serve.Stop()
		return fmt.Errorf("the model server couldn't start%s", reason)
	}
	var first int32
	for i := 0; i < 40; i++ {
		time.Sleep(500 * time.Millisecond)
		if serve.Healthy(ctx, cfg) {
			return nil
		}
		if serve.FatalInLog() != "" {
			return fail()
		}
		pids := serve.OwnPIDs(ctx)
		switch {
		case len(pids) > 0 && first == 0:
			first = pids[0]
		case len(pids) > 0 && !slices.Contains(pids, first):
			return fail() // crashed and was restarted
		case len(pids) > 0 && i >= 10:
			return nil // still loading
		case len(pids) == 0 && (first != 0 || i >= 20):
			return fail()
		}
	}
	return nil
}

// SetPort moves the server to another port; connected apps follow.
func (a *App) SetPort(ctx context.Context, port int) error {
	cfg, err := serve.Load()
	if err != nil {
		return errors.New("nothing is set up yet")
	}
	if port < 1024 || port > 65535 {
		return errors.New("pick a port between 1024 and 65535")
	}
	if port == cfg.Port {
		return nil
	}
	serve.Stop()
	time.Sleep(500 * time.Millisecond)
	if !serve.PortFree(port) {
		a.StartSync(ctx, cfg, nil)
		return fmt.Errorf("port %d is in use by another program", port)
	}
	integrate.NotePort(cfg.Port) // configs that still say the old port stay EZLM's
	cfg.Port = port
	if err := cfg.Save(); err != nil {
		return err
	}
	integrate.NotePort(port)
	a.rewriteApps(cfg)
	return a.StartSync(ctx, cfg, nil)
}

// serverReason picks the telling line from the server's log, if any.
// Swapped out in tests, which must not download llama.cpp or register a
// real service.
var (
	installEngine  = engine.Install
	installService = serve.Install
)

// restoreServer downloads llama.cpp again when its program has gone missing
// (an antivirus quarantined it, or someone tidied up EZLM's folder) instead of
// failing with "file does not exist".
func (a *App) restoreServer(ctx context.Context, cfg *serve.Config) error {
	a.emit(Event{Job: "start", Step: "start", State: "info", Msg: "The model server program was missing, so EZLM is downloading it again"})
	inst, err := installEngine(ctx, scan.Scan(ctx), nil)
	if err == nil && !fileExists(inst.Server) {
		err = errors.New("it disappeared again right after downloading")
	}
	if err != nil {
		return fmt.Errorf("EZLM's model server program is missing and couldn't be restored (%v). If an antivirus app removed it, allow the folder %s in that app, then press Start server", err, paths.Bin())
	}
	cfg.Server, cfg.EngineTag = inst.Server, inst.Tag
	if err := cfg.Save(); err != nil {
		return err
	}
	return installService(cfg)
}

func serverReason() string {
	log := ansi.ReplaceAllString(serve.TailLog(60), "")
	low := strings.ToLower(log)
	// The usual causes, in plain words.
	switch {
	case strings.Contains(low, "failed to open gguf file") || strings.Contains(low, "no such file"):
		return ": the model file is missing (it may have been moved or deleted). Pick the model again in Models to download it"
	case strings.Contains(low, "out of memory") || strings.Contains(low, "failed to allocate") || strings.Contains(low, "unable to allocate"):
		return ": there isn't enough free memory for this model right now. Close some apps (Free up memory) or pick a smaller model"
	case strings.Contains(low, "couldn't bind") || strings.Contains(low, "address already in use"):
		return ": another program is using its port. Close that program or restart your computer, then press Start server"
	}
	lines := strings.Split(log, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.ToLower(lines[i])
		if strings.Contains(l, "error") || strings.Contains(l, "failed") {
			t := strings.TrimSpace(lines[i])
			if r := []rune(t); len(r) > 160 {
				t = string(r[:160]) + "…"
			}
			return ": " + t
		}
	}
	return ""
}

// rewriteApps points connected apps at the current URL, key and model and
// returns the names of those it updated (failures show on their tiles).
func (a *App) rewriteApps(cfg *serve.Config) []string {
	return a.rewriteLinked(cfg, func(string) {})
}

func (a *App) isWarming() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.warming }

// Uninstall stops the server and deletes everything EZLM downloaded,
// including models stored on another drive (only folders EZLM created).
func (a *App) Uninstall() error {
	cfg, cerr := serve.Load()
	// The server must be fully gone first: Windows won't delete files (its
	// DLLs, the model it has mapped) while the process is still exiting.
	serve.Uninstall()
	for i := 0; i < 40 && len(serve.OwnPIDs(context.Background())) > 0; i++ {
		time.Sleep(250 * time.Millisecond)
	}
	dirs := modelDirs(nil)
	if cerr == nil {
		dirs = append(dirs, cfg.ModelsDir)
	}
	for _, d := range dirs {
		if isEZLMDir(d) && !within(d, paths.Home()) {
			if err := removeAllRetry(d); err != nil {
				return err
			}
		}
	}
	return removeAllRetry(paths.Home())
}

// removeAllRetry deletes a folder, retrying briefly while Windows releases
// files a just-exited process had open.
func removeAllRetry(dir string) error {
	var err error
	for i := 0; i < 20; i++ {
		if err = os.RemoveAll(dir); err == nil {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("couldn't delete %s: %v (close anything using it and try again)", dir, err)
}

func fileExists(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }

// cpuInstead is the way out when an AMD or Intel card's Vulkan driver can't
// load a model: the same model on the processor (no graphics device at all,
// so a broken driver can't get in the way), if it fits in memory there.
// NVIDIA (CUDA) and Mac (Metal) failures are real problems to report instead.
func cpuInstead(cfg *serve.Config, hw *scan.Hardware, p fit.Plan) (*serve.Config, bool) {
	if hw.Backend != scan.Vulkan || cfg.NGL == 0 {
		return nil, false
	}
	if p.GPUNeed+p.RAMNeed > fit.ComputeBudgets(hw, hw.RAMAvail).RAM {
		return nil, false
	}
	c := *cfg
	c.NGL, c.NCPUMoE = 0, 0
	c.ExtraArgs = append(slices.Clone(cfg.ExtraArgs), "--device", "none")
	return &c, true
}

// driverFailure: a load error that points at the graphics card (a crash,
// a failed allocation, a Vulkan error) rather than at the model file, the
// port or slowness, which would fail the same way on the processor.
func driverFailure(msg string) bool {
	l := strings.ToLower(msg)
	if strings.HasPrefix(l, serve.ErrStopped) {
		return true // quit without saying why: graphics drivers do that
	}
	for _, k := range []string{"did not finish loading", "address already in use", "couldn't bind"} {
		if strings.Contains(l, k) {
			return false
		}
	}
	for _, k := range []string{"vulkan", "vk::", "ggml_vk", "allocate", "out of memory", "device"} {
		if strings.Contains(l, k) {
			return true
		}
	}
	return false
}

// firstLine: an error's headline, without the log lines that follow it.
func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	s, _, _ = strings.Cut(s, ". Last log lines")
	return strings.TrimSuffix(strings.TrimSpace(s), ".")
}

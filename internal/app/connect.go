package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/picom-code/ezlm/internal/catalog"
	"github.com/picom-code/ezlm/internal/integrate"
	"github.com/picom-code/ezlm/internal/serve"
)

// endpointFor is what the app writers get to know about the deployed model.
func endpointFor(cfg *serve.Config) integrate.Endpoint {
	ep := integrate.Endpoint{BaseURL: cfg.LocalURL(), Key: cfg.Key, ModelID: cfg.ModelID, ModelName: cfg.ModelName, Ctx: cfg.Ctx, ToolCalls: cfg.ToolCalls}
	if p := cfg.Props; p != nil {
		ep.Reasoning = integrate.Reasoning{Levels: p.Levels, Thinking: p.Thinking}
		ep.Vision = p.Vision
	}
	return ep
}

// IntegrationLog is the end of an app's integration log (keys already
// removed), for "Show details".
func (a *App) IntegrationLog(id string, n int) []string {
	if _, ok := integrate.Lookup(id); !ok {
		return []string{}
	}
	lines := integrate.LogTail(id, n)
	if lines == nil {
		return []string{}
	}
	return lines
}

// Preflight runs an app's launch preflight against the deployed model
// (ezlm doctor apps): no window, no new chat.
func (a *App) Preflight(ctx context.Context, id string) error {
	cfg, err := serve.Load()
	if err != nil {
		return errors.New("set up a model first")
	}
	return integrate.Preflight(ctx, id, endpointFor(cfg), nil)
}

// Seams for tests, which must not start a real server or open windows.
var (
	serverHealthy = serve.Healthy
	serverWait    = serve.WaitHealthy
	serverStart   = func(a *App, ctx context.Context, cfg *serve.Config) error { return a.StartSync(ctx, cfg, nil) }
	fetchProps    = serve.FetchProps
	openApp       = integrate.Open
	loadCatalog   = catalog.Load
)

// serverGate makes sure EZLM's own server answers before an app is checked
// or opened, starting it when it isn't running, even when the user stopped
// it: asking for an app that needs the model is asking for the model. A
// server that can't start gives its reason at once, instead of a check that
// times out minutes later. StartSync may move the server to another port,
// so callers build the Endpoint after the gate.
func (a *App) serverGate(ctx context.Context, cfg *serve.Config, say func(string)) error {
	if !serverHealthy(ctx, cfg) {
		say("Starting your model server…")
		os.Remove(serve.StoppedFlag())
		if !a.isWarming() { // a Start already running would only be doubled
			if err := serverStart(a, ctx, cfg); err != nil {
				return err
			}
		}
		if err := serverWait(ctx, cfg, 10*time.Minute, nil); err != nil {
			return fmt.Errorf("your model server didn't start: %s. Press Start server on Overview to try again", firstLine(err.Error()))
		}
	}
	integrate.NotePort(cfg.Port)
	if cfg.Props == nil {
		a.refreshProps(ctx, cfg)
	}
	useCatalogPins(ctx)
	return nil
}

// refreshProps reads /props (reasoning levels, thinking) into the saved
// config, so the app writers know what the model's template accepts even
// while the server is down. A server that doesn't answer leaves it as it was.
func (a *App) refreshProps(ctx context.Context, cfg *serve.Config) {
	p, err := fetchProps(ctx, cfg)
	if err != nil {
		return
	}
	cfg.Props = p
	cfg.Save()
}

// useCatalogPins applies the remote catalog's tool pins, if it has any.
func useCatalogPins(ctx context.Context) {
	if cat, _ := loadCatalog(ctx); cat != nil {
		integrate.UseCatalogPins(cat.Tools)
	}
}

// rewriteLinked points every app whose config has EZLM's entry at the
// current URL, key, model and context, and returns the names of those it
// updated. An app whose context floor the model is below is left alone (it
// shows needs_64k instead). A failed rewrite is kept as that app's Detail
// in Status and said through say.
func (a *App) rewriteLinked(cfg *serve.Config, say func(string)) []string {
	ep := endpointFor(cfg)
	var names []string
	for _, t := range integrate.Targets(&ep) {
		if !t.Linked {
			continue
		}
		if t.MinCtx > 0 && ep.Ctx < t.MinCtx {
			a.setAppErr(t.ID, "")
			say(integrate.NeedsCtxDetail(t.Name, t.MinCtx))
			continue
		}
		if err := integrate.Rewrite(t.ID, ep); err != nil {
			a.setAppErr(t.ID, err.Error())
			say(t.Name + ": " + err.Error())
			continue
		}
		a.setAppErr(t.ID, "")
		names = append(names, t.Name)
	}
	return names
}

// setAppErr records (msg != "") or clears why EZLM couldn't update an app.
func (a *App) setAppErr(id, msg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if msg == "" {
		delete(a.appErrs, id)
		return
	}
	if a.appErrs == nil {
		a.appErrs = map[string]string{}
	}
	a.appErrs[id] = msg
}

// withAppErrs shows a failed rewrite on the app's tile: its config still
// holds the old settings, so it needs an update whatever Detect saw.
func (a *App) withAppErrs(ts []integrate.Target) []integrate.Target {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range ts {
		msg, ok := a.appErrs[ts[i].ID]
		if !ok || ts[i].State == integrate.StateNeeds64K || ts[i].State == integrate.StateNotInstalled {
			continue
		}
		if ts[i].State == integrate.StateConnected {
			ts[i].State, ts[i].Connected = integrate.StateNeedsUpdate, false
		}
		ts[i].Detail = msg
	}
	return ts
}

// restartNote asks the user to reopen app windows that were already open
// when their key or address changed: a running app keeps the old settings.
func restartNote(names []string) string {
	if len(names) == 0 {
		return ""
	}
	list := names[0]
	if len(names) > 1 {
		list = strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
	}
	return "If " + list + " is already open, close it and open it again so it uses the new settings."
}

// OpenSessionsNote is restartNote for the apps connected right now (the
// CLI prints it after moving the server).
func OpenSessionsNote() string {
	cfg, err := serve.Load()
	if err != nil {
		return ""
	}
	ep := endpointFor(cfg)
	var names []string
	for _, t := range integrate.Targets(&ep) {
		if t.Linked {
			names = append(names, t.Name)
		}
	}
	return restartNote(names)
}

// ctxFor is the context to give an app with a floor of minCtx tokens: the
// next power of two (64,000 → 65,536).
func ctxFor(minCtx int) int {
	n := 1024
	for n < minCtx {
		n *= 2
	}
	return n
}

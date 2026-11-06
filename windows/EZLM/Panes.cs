using Microsoft.UI;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Documents;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media;
using Windows.System;
using static EZLM.Ui;

namespace EZLM;

/// A list of model options. Cards are built once per scan; changing the
/// selection only restyles two of them.
sealed class PlanList
{
    public readonly StackPanel Panel = Col(8);
    readonly List<(Plan plan, Border card, Button? act)> cards = new();
    readonly Func<Plan, (string label, bool enabled)>? actionFor;
    string? selected;

    /// With onAction, the selected card shows its own button (e.g. "Switch to X"),
    /// and double-click acts directly, so nobody has to scroll to find it.
    public PlanList(List<Plan> plans, string? recommendedId, string? runningId, Hardware hw, Action<string> pick,
        Func<Plan, (string label, bool enabled)>? actionFor = null, Action<Plan>? onAction = null)
    {
        this.actionFor = actionFor;
        var maxScore = plans.Count > 0 ? plans.Max(x => x.Model.Score) : 1;
        foreach (var p in plans)
        {
            var card = Card(p, maxScore, p.Model.Id == recommendedId, p.Model.Id == runningId, hw);
            Button? act = null;
            if (onAction != null)
            {
                act = Btn("", Kind.Primary, () => onAction(p));
                act.Visibility = Visibility.Collapsed;
                act.Margin = new Thickness(0, 6, 0, 0);
                ((StackPanel)card.Child).Children.Add(act);
                card.DoubleTapped += (_, _) => { if (actionFor?.Invoke(p).enabled == true) onAction(p); };
            }
            card.Tapped += (_, _) => pick(p.Model.Id);
            cards.Add((p, card, act));
            Panel.Children.Add(card);
        }
    }

    public void Select(string? id)
    {
        foreach (var (p, card, act) in cards)
        {
            bool on = p.Model.Id == id, was = p.Model.Id == selected;
            if (act != null && on && actionFor != null)
            {
                var (label, enabled) = actionFor(p);
                ((TextBlock)act.Content).Text = label;
                Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(act, label.Replace("↵", "").Trim());
                act.IsEnabled = enabled;
            }
            if (on == was && id == selected) continue;
            card.BorderBrush = B(on ? Signal : Faint);
            card.Background = B(on ? C(0xb9a8ff, 18) : Colors.Transparent);
            if (act != null) act.Visibility = on ? Visibility.Visible : Visibility.Collapsed;
            if (on && !was) card.StartBringIntoView(new BringIntoViewOptions { AnimationDesired = false, VerticalAlignmentRatio = 0.3 });
        }
        selected = id;
    }

    static Border Card(Plan p, double maxScore, bool recommended, bool running, Hardware hw)
    {
        var head = new Grid();
        head.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        head.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        head.Children.Add(Row(9, Marks.Logo(p.Model.Vendor, 18), Txt(p.Model.Name, 12, Ink, true, false), Marks.Origin(p.Model.Country)));
        var tags = Row(8);
        if (running) tags.Children.Add(Txt("Running", 10, Ok, true));
        else if (p.Downloaded) tags.Children.Add(Txt("On this computer", 10, Ok));
        if (recommended) tags.Children.Add(Txt("Recommended", 10, Signal));
        else if (p.Speed == "slow") tags.Children.Add(Txt("Slow", 10, Bad));
        if (!p.DiskOk && !p.Downloaded) tags.Children.Add(Txt("Needs space", 10, Gold));
        Grid.SetColumn(tags, 1); head.Children.Add(tags);
        var smarts = new BlockBar(6); smarts.SetFill(p.Model.Score / maxScore, Ink);
        var speed = new BlockBar(6); speed.SetFill(Math.Min(1, Math.Log(1 + p.TokPerSec) / Math.Log(151)), p.Speed == "slow" ? Bad : Ok);
        var bars = new Grid { ColumnSpacing = 10 };
        foreach (var w in new[] { 46.0, 1, 40, 1 }) bars.ColumnDefinitions.Add(new ColumnDefinition { Width = w > 1 ? new GridLength(w) : new GridLength(1, GridUnitType.Star) });
        void Put(UIElement e, int c) { Grid.SetColumn((FrameworkElement)e, c); bars.Children.Add(e); }
        Put(Txt("Smarts", 10, Dim), 0); Put(smarts, 1); Put(Txt("Speed", 10, Dim), 2); Put(speed, 3);
        var card = new Border
        {
            Padding = new Thickness(10), BorderThickness = new Thickness(1), BorderBrush = B(Faint), Background = B(Colors.Transparent),
            Child = Col(6, head, Txt(p.How(hw.Unified) + $", ~{p.Words} words/s", 10, Dim, false, false), Txt(p.CtxLine, 10, Dim, false, false), Txt(p.QuantLine, 10, Faint, false, false), bars),
        };
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(card, p.Model.Name);
        return card;
    }
}

public sealed partial class Shell
{
    // MARK: Overview

    TextBlock? ovSpeed, ovTok, ovUp, ovQueue;
    StackPanel? ovMem, testPanel;
    ActivityView? activity;
    readonly Dictionary<string, LiveBarView> liveBars = new();
    List<TestResult> tests = new();
    bool testing;
    double lastSpeed;

    void Overview(StackPanel b)
    {
        var s = status; var c = s?.Config;
        if (c == null || s?.Deployed != true)
        {
            Header(b, "Nothing is set up yet", "Pick a model and EZLM installs everything for you.");
            b.Children.Add(Btn("Choose a model  ↵", Kind.Primary, () => _ = RunScan()));
            return;
        }
        var st = ServerState;
        var head = new Grid { Margin = new Thickness(0, 0, 0, 18) };
        head.Children.Add(Col(4, Txt(c.ModelName, 22, Ink, true), Txt($"{c.Quant}, {c.Ctx / 1024}k context", 12, Dim)));
        var badge = Row(8, new Border { Width = 9, Height = 9, CornerRadius = new CornerRadius(5), Background = B(st == 2 ? Ok : st == 1 ? Gold : Bad), VerticalAlignment = VerticalAlignment.Center },
            Txt(st == 2 ? "Live" : st == 1 ? "Loading model…" : "Stopped", 13, st == 2 ? Ok : st == 1 ? Gold : Bad, true));
        badge.HorizontalAlignment = HorizontalAlignment.Right; badge.VerticalAlignment = VerticalAlignment.Top; head.Children.Add(badge);
        b.Children.Add(head);

        if (updateInfo is { Available: true, Latest: { } nu })
        {
            b.Children.Add(new Border
            {
                BorderBrush = B(C(0x8fe3b5, 140)), BorderThickness = new Thickness(1), Background = B(C(0x8fe3b5, 12)), Padding = new Thickness(14, 10, 14, 10), Margin = new Thickness(0, 0, 0, 14),
                Child = Row(14, Txt($"EZLM {nu.Version} is available", 13, Ink, true).Also(t => t.VerticalAlignment = VerticalAlignment.Center),
                    Btn(updateBusy ? "Updating…" : "Update now", Kind.Primary, () => _ = StartUpdate()).Also(x => x.IsEnabled = !updateBusy && !installRunning),
                    Btn("What's new", Kind.Normal, () => Go("Settings"))),
            });
        }
        if (s.Unmanaged)
        {
            b.Children.Add(new Border
            {
                BorderBrush = B(C(0xe8c872, 150)), BorderThickness = new Thickness(1), Padding = new Thickness(14, 12, 14, 12), Margin = new Thickness(0, 0, 0, 14),
                Child = Col(6, Txt("EZLM can't control this server right now", 13, Gold, true),
                    Txt("It was started with administrator rights, so EZLM can't stop it or switch models. It keeps working for your apps. Restart your computer (or end \"llama-server\" in Task Manager) and EZLM takes over again.", 12, Dim)),
            });
        }
        if (c?.CpuFallback is { Length: > 0 } why && !s.Unmanaged)
        {
            b.Children.Add(new Border
            {
                BorderBrush = B(C(0xe8c872, 150)), BorderThickness = new Thickness(1), Padding = new Thickness(14, 12, 14, 12), Margin = new Thickness(0, 0, 0, 14),
                Child = Col(8, Txt("Running on the processor", 13, Gold, true),
                    Txt($"Your graphics card couldn't load this model ({why}), so it runs on the processor, slower than it could be. A newer graphics driver may fix this.", 12, Dim),
                    Row(10, Btn("Try the graphics card again", Kind.Normal, () => _ = Reapply(c, c.Quant, "Trying your graphics card again")).Also(x => x.IsEnabled = !installRunning))),
            });
        }
        if (s.Tune is { } tune && st == 2 && !installRunning && !s.Unmanaged)
        {
            var card = new Border
            {
                BorderBrush = B(C(0xb9a8ff, 140)), BorderThickness = new Thickness(1), Background = B(C(0xb9a8ff, 14)), Padding = new Thickness(14, 12, 14, 12), Margin = new Thickness(0, 0, 0, 14),
                Child = Col(8, Txt("A faster setup is available for this model", 13, Ink, true), Txt(tune.Why, 12, Dim),
                    Row(10, Btn("Apply it", Kind.Primary, () => _ = ApplyTune(c, tune)), Txt("Restarts your server, about a minute. Nothing is downloaded.", 11, Dim).Also(t => t.VerticalAlignment = VerticalAlignment.Center))),
            };
            b.Children.Add(card);
        }

        activity = new ActivityView(() => _ = StartServer()) { StartError = s.StartError };
        b.Children.Add(activity.Root);

        ovSpeed = Txt("–", 22, Ink, true); ovTok = Txt("0", 22, Ink, true); ovUp = Txt("–", 22, Ink, true); ovQueue = Txt("0", 22, Ink, true);
        var tiles = new Grid { ColumnSpacing = 10, Margin = new Thickness(0, 0, 0, 26) };
        int i = 0;
        foreach (var (label, val, unit) in new[] { ("Writing speed", ovSpeed, "tok/s"), ("Generated", ovTok, "tokens"), ("Waiting", ovQueue, "requests"), ("Up for", ovUp, "") })
        {
            tiles.ColumnDefinitions.Add(new ColumnDefinition());
            var tile = new Border { BorderBrush = B(Rule), BorderThickness = new Thickness(1), Padding = new Thickness(12), MinHeight = 80, Child = Col(6, Txt(label, 11, Dim), Row(5, val, Txt(unit, 11, Dim).Also(t => t.VerticalAlignment = VerticalAlignment.Bottom))) };
            Grid.SetColumn(tile, i++); tiles.Children.Add(tile);
        }
        b.Children.Add(tiles);

        b.Children.Add(Section("Connect with"));
        b.Children.Add(Endpoint(s.LocalUrl ?? "", c.Key, false));
        var connected = s.Targets.Where(t => t.Supported && t.Shown == "connected").ToList();
        if (connected.Count == 0) b.Children.Add(Txt("Not connected to any apps yet.", 12, Dim).Also(t => t.Margin = new Thickness(0, 10, 0, 0)));
        else
        {
            // "Connected to" and each app with its logo, as on the Mac and the web page.
            var line = new FlowPanel { Spacing = 14, LineSpacing = 6, Margin = new Thickness(0, 12, 0, 0) };
            line.Children.Add(Txt("Connected to", 12, Dim, wrap: false).Also(x => x.VerticalAlignment = VerticalAlignment.Center));
            foreach (var t in connected)
                line.Children.Add(Row(6, Marks.AppLogo(t.LogoKey, 20), Txt(t.Name, 12, Ink, wrap: false).Also(x => x.VerticalAlignment = VerticalAlignment.Center)));
            Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(line, $"Connected to {AppText.And(connected.Select(t => t.Name).ToList())}.");
            b.Children.Add(line);
        }
        b.Children.Add(Section("Use it"));
        b.Children.Add(LaunchButtons(c));
        b.Children.Add(Row(12,
            Btn("Connect another app", Kind.Small, () => Go("Connect")),
            Btn("Manage keys", Kind.Small, () => Go("Keys"))).Also(r => r.Margin = new Thickness(0, 10, 0, 0)));

        var testHead = new Grid();
        testHead.Children.Add(Section("Model check"));
        var tb = Btn(testing ? "Testing…" : tests.Count == 0 ? "Test model" : "Test again", Kind.Small, () => _ = RunTests());
        tb.IsEnabled = !testing && st == 2; tb.HorizontalAlignment = HorizontalAlignment.Right; tb.VerticalAlignment = VerticalAlignment.Bottom; testHead.Children.Add(tb);
        b.Children.Add(testHead);
        testPanel = Col(4);
        RenderTests();
        b.Children.Add(testPanel);

        b.Children.Add(Section("Memory right now"));
        ovMem = Col(0); liveBars.Clear();
        b.Children.Add(ovMem);

        b.Children.Add(Section("Server"));
        b.Children.Add(Row(12,
            st != 0 ? Btn("Stop server", Kind.Normal, async () => { try { await engine.Post("stop"); } catch (Exception ex) { note = ex.Message; } await Task.Delay(600); status = await engine.Get<Status>("status"); live = null; ShowSection(); })
                    : Btn("Start server", Kind.Primary, () => _ = StartServer()),
            Btn("Change model", Kind.Normal, () => Go("Models")),
            Btn("Free up memory", Kind.Normal, () => _ = ModelMode(ShowSection))));
        if (note != "") b.Children.Add(Txt(note, 12, Dim).Also(t => t.Margin = new Thickness(0, 8, 0, 0)));
    }

    async Task StartServer()
    {
        note = "Starting. Loading the model can take a minute.";
        live = new Live { Starting = true, Phase = "idle" };
        ShowSection();
        try { await engine.Post("start"); } catch (Exception ex) { note = ex.Message; live = null; ShowSection(); }
    }

    Task ApplyTune(ServerConfig c, TuneHint t) => Reapply(c, t.Quant, "Applying the faster setup");

    /// Sets the running model up again with today's plan (a faster setup, or
    /// the graphics card again after it couldn't load the model).
    async Task Reapply(ServerConfig c, string quant, string title)
    {
        if (installRunning) return;
        NewSteps(true);
        installTitle = title;
        fillName = c.ModelName; fillGpuNeed = 0; fillRamNeed = 0; // the new plan's numbers arrive with the live feed
        installRunning = true;
        try { await engine.Post("use", new { model_id = c.ModelId, quant, pref = "balanced" }); }
        catch (Exception ex) { installRunning = false; note = ex.Message; ShowSection(); return; }
        logo?.SetBusy(true);
        await ShowInstallDialog();
    }

    /// Opens llama.cpp's built-in chat. It asks for the key, so it's copied first.
    async Task OpenChat(ServerConfig c)
    {
        Copy(c.Key);
        note = "Your key is copied: if the chat asks for an API key, paste it in Settings.";
        ShowSection();
        await Launcher.LaunchUriAsync(new Uri($"http://127.0.0.1:{c.Port}/"));
    }

    /// One line that always says what the model is doing right now.
    sealed class ActivityView
    {
        public readonly StackPanel Root;
        readonly Border mark = new() { Width = 16 };
        readonly TextBlock text = Txt("", 13), detail = Txt("", 12, Dim);
        readonly BlockBar bar = new(5) { Width = 420, HorizontalAlignment = HorizontalAlignment.Left };
        string phase = "";

        readonly Button start;
        readonly StackPanel fillHost = Col(0);

        public ActivityView(Action onStart)
        {
            start = Btn("Start server", Kind.Primary, onStart);
            start.Margin = new Thickness(0, 6, 0, 0);
            fillHost.Margin = new Thickness(0, 8, 0, 0);
            Root = Col(6, Row(10, mark, text), detail, bar, start, fillHost);
            Root.Margin = new Thickness(0, 0, 0, 20);
            Root.Padding = new Thickness(14, 12, 14, 12);
            Root.BorderBrush = B(Rule); Root.BorderThickness = new Thickness(1);
        }

        /// Shows (or removes) the memory-filling panel; it survives pane rebuilds.
        public void ShowFill(MemoryFillView? f)
        {
            if (f == null) { fillHost.Children.Clear(); return; }
            if (f.Root.Parent == fillHost) return;
            if (f.Root.Parent is Panel old) old.Children.Remove(f.Root);
            fillHost.Children.Clear();
            fillHost.Children.Add(f.Root);
        }

        public string? StartError;

        public void Update(Live l, int serverState)
        {
            var ph = serverState == 1 ? "loading" : serverState == 0 ? "stopped" : l.Phase ?? "idle";
            start.Visibility = ph == "stopped" ? Visibility.Visible : Visibility.Collapsed;
            if (ph != phase)
            {
                phase = ph;
                mark.Child = ph is "reading" or "writing" or "loading" ? new Spinner(ph == "writing" ? Ok : Signal) : Txt(ph == "stopped" ? "■" : "●", 11, ph == "stopped" ? Bad : Ok);
            }
            switch (ph)
            {
                case "reading":
                    var pct = l.PromptTotal > 0 ? (double)l.PromptDone / l.PromptTotal : 0;
                    text.Text = "Reading a prompt";
                    detail.Text = $"{l.PromptDone:N0} of {l.PromptTotal:N0} tokens ({pct:P0})" + (l.Queued > 0 ? $", {l.Queued} more waiting" : "") + ". Long prompts from coding agents take a moment before the reply starts.";
                    bar.Visibility = Visibility.Visible; bar.SetFill(pct, Signal);
                    break;
                case "writing":
                    text.Text = "Writing a reply";
                    detail.Text = $"{l.Written:N0} tokens so far at {(int)l.TokPerSec} tok/s" + (l.Queued > 0 ? $", {l.Queued} more waiting" : "");
                    bar.Visibility = Visibility.Visible; bar.Working(Ok);
                    break;
                case "loading":
                    text.Text = "Loading the model into memory";
                    detail.Text = "This takes a little while after a restart. Requests work as soon as it's done.";
                    bar.Visibility = Visibility.Collapsed;
                    break;
                case "stopped":
                    text.Text = string.IsNullOrEmpty(StartError) ? "The server is stopped" : "The server couldn't start";
                    detail.Text = string.IsNullOrEmpty(StartError) ? "Apps can't reach your model until you start it again." : StartError + ". Try Start server again; if it keeps happening, use Copy details for support in Logs.";
                    bar.Visibility = Visibility.Collapsed;
                    break;
                default:
                    text.Text = "Ready";
                    detail.Text = "Waiting for requests from your apps.";
                    bar.Visibility = Visibility.Collapsed;
                    break;
            }
        }
    }

    MemoryFillView? ovFill;
    DateTime ovFillDoneAt;

    void UpdateOverviewLive()
    {
        if (live is not { } l) return;
        activity?.Update(l, ServerState);
        // The pour animation for loads that happen outside the install flow.
        if (ServerState == 1)
        {
            ovFill ??= new MemoryFillView(status?.Config?.ModelName ?? "your model");
            ovFill.Feed(l);
            activity?.ShowFill(ovFill);
        }
        else if (ovFill != null)
        {
            if (ovFillDoneAt == default) { if (ServerState == 2) ovFill.Complete(l); ovFillDoneAt = DateTime.Now; }
            if ((DateTime.Now - ovFillDoneAt).TotalSeconds > 5) { ovFill = null; ovFillDoneAt = default; activity?.ShowFill(null); }
            else activity?.ShowFill(ovFill);
        }
        if (ovSpeed == null) return;
        if (l.Phase == "writing" && l.TokPerSec > 0) lastSpeed = l.TokPerSec;
        ovSpeed.Text = lastSpeed > 0 ? $"{(int)lastSpeed}" : "–";
        ovTok!.Text = l.Tokens >= 1_000_000 ? $"{l.Tokens / 1e6:0.0}M" : l.Tokens >= 1000 ? $"{l.Tokens / 1e3:0.0}k" : $"{l.Tokens}";
        ovQueue!.Text = $"{l.Queued}";
        ovUp!.Text = l.UptimeS > 0 ? Fmt.Uptime(l.UptimeS) : "–";
    }

    void UpdateMemBars()
    {
        if (live is not { } l || ovMem == null) return;
        void Bar(string key, string label, ulong total, ulong free)
        {
            if (!liveBars.TryGetValue(key, out var lb) || lb.Root.Parent != ovMem)
            {
                lb = new LiveBarView(label);
                liveBars[key] = lb;
                ovMem.Children.Add(lb.Root);
            }
            lb.Update(total, free);
        }
        if (l.Unified) Bar("mem", "Memory, shared with graphics", l.RamTotal, l.RamAvail);
        else { if (l.GpuTotal > 0) Bar("gpu", "Graphics memory", l.GpuTotal, l.GpuFree); Bar("ram", "System memory", l.RamTotal, l.RamAvail); }
    }

    sealed class LiveBarView
    {
        public readonly StackPanel Root;
        readonly TextBlock used = Txt("", 12), of = Txt("", 12, Dim);
        readonly BlockBar bar = new(14);
        public LiveBarView(string label)
        {
            var head = new Grid(); head.Children.Add(Txt(label, 13, Dim));
            var r = Row(0, used, of); r.HorizontalAlignment = HorizontalAlignment.Right; head.Children.Add(r);
            Root = Col(6, head, bar); Root.Margin = new Thickness(0, 0, 0, 14);
        }
        public void Update(ulong total, ulong free)
        {
            var u = total > free ? total - free : 0;
            used.Text = Fmt.Gb(u); of.Text = $" used of {Fmt.Gb(total)}";
            bar.SetFill((double)u / Math.Max(total, 1), (double)u / Math.Max(total, 1) > 0.95 ? Gold : Signal);
        }
    }

    async Task RunTests()
    {
        if (testing) return;
        tests.Clear(); testing = true; logo?.SetBusy(true); ShowSection();
        try { await engine.Post("test"); } catch (Exception ex) { testing = false; note = ex.Message; ShowSection(); }
    }

    void RenderTests()
    {
        if (testPanel == null) return;
        testPanel.Children.Clear();
        if (tests.Count == 0 && !testing) testPanel.Children.Add(Txt("Runs five real requests: speed, reasoning, tool calling, following instructions, and long-context recall.", 12, Dim));
        foreach (var t in tests) testPanel.Children.Add(Row(10, new Border { Width = 16, Child = Txt(t.Pass ? "✓" : "✗", 13, t.Pass ? Ok : Bad) }, Col(2, Txt(t.Name), Txt($"{t.Detail}  ({t.Secs:0.0}s)", 12, Dim))));
        if (testing)
        {
            var names = new[] { "Speed", "Reasoning", "Tool calling", "Following instructions", "Long-context recall" };
            testPanel.Children.Add(Row(10, new Border { Width = 16, Child = new Spinner() }, Txt(names[Math.Min(tests.Count, 4)])));
        }
        else if (tests.Count > 0)
            testPanel.Children.Add(Txt($"{tests.Count(t => t.Pass)} of {tests.Count} passed", 12, tests.All(t => t.Pass) ? Ok : Gold).Also(t => t.Margin = new Thickness(26, 4, 0, 0)));
    }

    // MARK: Models

    PlanList? modelsList;
    StackPanel? modelsLib;

    void ModelsPane(StackPanel b)
    {
        Header(b, "Models", "Switch any time: your URL, keys and connected apps stay the same.");
        b.Children.Add(Section("On this computer"));
        modelsLib = Col(0);
        b.Children.Add(modelsLib);
        RenderInstalled();
        var lib = modelsLib;
        _ = RefreshInstalled().ContinueWith(_ => Ui(() => { if (modelsLib == lib) RenderInstalled(); }));

        b.Children.Add(Section("Get another model"));
        b.Children.Add(PrefPicker());
        if (scan == null)
        {
            b.Children.Add(Row(10, new Spinner(), Txt("Comparing models against your memory")).Also(r => r.Margin = new Thickness(0, 14, 0, 0)));
            _ = RefreshScan().ContinueWith(_ => Ui(() => { if (section == "Models" && scan != null) ShowSection(); }));
            return;
        }
        var current = status?.Deployed == true ? status.Config?.ModelId : null;
        var plans = VisiblePlans;
        if (chosenId == null || plans.All(p => p.Model.Id != chosenId)) chosenId = current ?? scan.Recommended?.Model.Id;
        if (note != "") b.Children.Add(Txt(note, 12, Bad).Also(t => t.Margin = new Thickness(0, 10, 0, 0)));
        b.Children.Add(Txt("Click a model to see its button, double-click or press ↵ to switch. Your current model keeps answering while a new one downloads.", 11, Dim).Also(t => t.Margin = new Thickness(0, 10, 0, 0)));
        modelsList = new PlanList(plans, scan.Recommended?.Model.Id, current, scan.Hw, id => { chosenId = id; UpdateModelsSelection(); },
            p => SwitchLabel(p, current), p => { chosenId = p.Model.Id; _ = Deploy(); });
        b.Children.Add(modelsList.Panel.Also(s => s.Margin = new Thickness(0, 12, 0, 0)));
        b.Children.Add(Btn("Scan again", Kind.Normal, async () => { scan = null; await RefreshScan(); ShowSection(); }).Also(r => r.Margin = new Thickness(0, 16, 0, 0)));
        UpdateModelsSelection();
    }

    void UpdateModelsSelection() => modelsList?.Select(chosenId);

    (string, bool) SwitchLabel(Plan p, string? current) =>
        p.Model.Id == current ? ("Running now", false)
        : installRunning ? ("Busy with another change…", false)
        : p.Downloaded ? ($"Switch to {p.Model.Name}  ↵ (no download)", true)
        : p.Partial > 0 ? ($"Resume download and switch  ↵ ({Fmt.Gb(p.Download)} left)", true)
        : ($"Download and switch  ↵ ({Fmt.Gb(p.Download > 0 ? p.Download : p.Quant.Bytes)})", true);

    async Task RefreshInstalled()
    {
        try { installed = (await engine.Post<InstalledList>("installed", new { pref })).Models; } catch (Exception ex) { Log(ex); }
    }

    void RenderInstalled()
    {
        var lib = modelsLib;
        if (lib == null) return;
        lib.Children.Clear();
        if (installed.Count == 0) { lib.Children.Add(Txt("Models you download stay here so you can switch back without downloading again.", 12, Dim)); return; }
        lib.Children.Add(Hairline());
        foreach (var im in installed)
        {
            var g = new Grid { Padding = new Thickness(0, 10, 0, 10), ColumnSpacing = 12 };
            g.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
            g.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
            g.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
            var logoEl = Marks.Logo(im.Vendor, 22); ((FrameworkElement)logoEl).VerticalAlignment = VerticalAlignment.Center; g.Children.Add(logoEl);
            var detail = $"{im.Quant}, {Fmt.Gb(im.Bytes)}" + (im.Plan is { } p ? $", ~{p.Words} words/s, {p.Ctx / 1024}k context" : ", doesn't fit in free memory right now");
            var mid = Col(3, Txt(im.Name, 13, Ink, true), Txt(detail, 11, Dim, false, false)); Grid.SetColumn(mid, 1); g.Children.Add(mid);
            var acts = Row(8, Marks.Origin(im.Country));
            if (im.Running) acts.Children.Add(new Border { BorderBrush = B(C(0x8fe3b5, 128)), BorderThickness = new Thickness(1), Padding = new Thickness(8, 3, 8, 3), Child = Txt("Running", 11, Ok, true) });
            else
            {
                acts.Children.Add(Btn("Switch", Kind.Small, () => _ = Use(im)).Also(x => x.IsEnabled = im.Plan != null && !installRunning));
                acts.Children.Add(Btn("Remove", Kind.Small, async () =>
                {
                    var d = Dialog($"Remove {im.Name}?", $"Frees {Fmt.Gb(im.Bytes)}. You can download it again any time.", "Remove");
                    if (await ShowDialog(d) != ContentDialogResult.Primary) return;
                    try { await engine.Post("remove", new { model_id = im.ModelId, quant = im.Quant }); } catch (Exception ex) { note = ex.Message; }
                    await RefreshInstalled(); scan = null; ShowSection();
                }));
            }
            acts.VerticalAlignment = VerticalAlignment.Center; Grid.SetColumn(acts, 2); g.Children.Add(acts);
            lib.Children.Add(g); lib.Children.Add(Hairline());
        }
    }

    async Task Use(InstalledModel im)
    {
        if (installRunning) return;
        NewSteps(true);
        chosenId = im.ModelId;
        installTitle = $"Switching to {im.Name}";
        fillName = im.Name; fillGpuNeed = im.Plan is { } ip && ip.Mode != "cpu" ? ip.GpuNeed : 0; fillRamNeed = im.Plan?.RamNeed ?? 0;
        installRunning = true;
        try { await engine.Post("use", new { model_id = im.ModelId, quant = im.Quant, pref }); }
        catch (Exception ex) { installRunning = false; note = ex.Message; if (section == "Chat") Toast(ex.Message, Bad); ShowSection(); return; }
        logo?.SetBusy(true);
        await ShowInstallDialog();
    }

    // MARK: System

    void SystemPane(StackPanel b)
    {
        Header(b, "System", "What this machine has, and what's using it. Memory updates live.");
        b.Children.Add(new Border
        {
            BorderBrush = B(C(0x8fe3b5, 110)), BorderThickness = new Thickness(1), Padding = new Thickness(14, 12, 14, 12), Margin = new Thickness(0, 0, 0, 20),
            Child = Col(8, Txt("Model mode", 13, Ink, true),
                Txt("Close apps you're not using so your AI gets more memory, especially graphics memory. EZLM asks about each app; nothing closes without your OK.", 12, Dim),
                Btn("Start Model mode", Kind.Primary, () => _ = ModelMode(() => { scan = null; ShowSection(); }))),
        });
        if (scan == null) { b.Children.Add(Row(10, new Spinner(), Txt("Reading your hardware"))); _ = RefreshScan().ContinueWith(_ => Ui(() => { if (section == "System" && scan != null) ShowSection(); })); return; }
        var hw = scan.Hw;
        var engines = new Dictionary<string, string> { ["cuda"] = "NVIDIA CUDA", ["metal"] = "Apple Metal", ["vulkan"] = "Vulkan", ["rocm"] = "AMD ROCm", ["cpu"] = "Processor only" };
        b.Children.Add(Fact("System", hw.OsName ?? hw.Os));
        b.Children.Add(Fact("Processor", $"{hw.CpuName}, {hw.Cores} threads"));
        b.Children.Add(Fact("Graphics", hw.Unified ? "Built into the chip, shares system memory" : (hw.Gpus ?? new()).Count == 0 ? "None found" : string.Join("\n", hw.Gpus!.Select(g => g.Name + (g.Driver != null ? $", driver {g.Driver}" : "")))));
        b.Children.Add(Fact("Engine", engines.GetValueOrDefault(hw.Backend, hw.Backend)));
        b.Children.Add(Fact("Storage", $"{Fmt.Gb(hw.DiskFree)} free" + (hw.ModelsDir != null ? $" in {hw.ModelsDir}" : "")).Also(f => f.Margin = new Thickness(0, 0, 0, 20)));
        ovMem = Col(0); liveBars.Clear(); b.Children.Add(ovMem);
        var list = hw.GpuProcs is { Count: > 0 } ? hw.GpuProcs : hw.TopMem ?? new();
        if (list.Count > 0)
        {
            b.Children.Add(Section(hw.GpuProcs is { Count: > 0 } ? "Already using your graphics card" : "Using the most memory right now"));
            foreach (var p in list) b.Children.Add(ProcRow(p).Also(r => ((FrameworkElement)r).Margin = new Thickness(0, 4, 0, 4)));
        }

        b.Children.Add(Section("Remove EZLM's models"));
        b.Children.Add(Txt("Stops your model, removes it from startup, and deletes the models EZLM downloaded plus its settings and keys. To remove the app itself, use Installed apps in Windows Settings.", 12, Dim));
        b.Children.Add(Btn("Remove models and settings", Kind.Danger, async () =>
        {
            var d = Dialog("Remove EZLM's models and settings?", "Apps you connected will stop working until you set EZLM up again. This can't be undone.", "Remove everything");
            if (await ShowDialog(d) != ContentDialogResult.Primary) return;
            try { await engine.Post("uninstall"); } catch (Exception ex) { note = ex.Message; ShowSection(); return; }
            status = null; live = null; installed.Clear(); scan = null; tests.Clear(); chooseBuiltFor = null;
            await RunScan();
        }).Also(x => x.Margin = new Thickness(0, 10, 0, 0)));
        if (note != "") b.Children.Add(Txt(note, 12, Dim).Also(t => t.Margin = new Thickness(0, 8, 0, 0)));
    }

    // MARK: Connect

    readonly HashSet<string> selectedApps = new();
    readonly Dictionary<string, (string text, Windows.UI.Color color, bool working)> appState = new();
    bool makeDefault = true;
    bool connecting;

    void ConnectPane(StackPanel b)
    {
        Header(b, "Connect", "Use your model inside these apps. EZLM installs them if needed, adds itself as a provider, and checks that it answers.");
        var tiles = new TilePanel { Spacing = 12 };
        bool busy = appState.Values.Any(v => v.working);
        foreach (var t in status?.Targets ?? new())
        {
            bool sel = selectedApps.Contains(t.Id);
            var h64 = t.Id == "hermes" ? status?.Hermes64 : null;
            var shown = AppText.Status(t);
            (string text, Windows.UI.Color color, bool working) st = appState.TryGetValue(t.Id, out var s) ? s
                : string.IsNullOrEmpty(t.State) && t.Connected && h64 != null ? ("Connected, but it needs a 64K context: connect again to give the model room", Gold, false)
                : (shown.Text, ToneColor(shown.Tone), false);
            string? tradeoff = h64 == null ? null
                : !h64.Fits ? $"Hermes needs a 64K context, which {status?.Config?.ModelName} can't have on this computer. Switch to a smaller model to use it with Hermes."
                : h64.TokPerSec >= 0.75 * h64.NowTokPerSec ? "Hermes needs a 64K context; EZLM gives the model the extra room when you connect (same speed)."
                : $"Hermes needs a 64K context. With it, {status?.Config?.ModelName} would write about {Math.Max(1, (int)(h64.TokPerSec * 0.75))} words/s instead of {Math.Max(1, (int)(h64.NowTokPerSec * 0.75))}, for every app.";
            // Hermes when the model can't have 64K, and any app the model can't use tools with, can't be picked.
            bool blocked = h64 is { Fits: false } || t.Shown == "needs_tools";
            bool tooBig = h64 is { Fits: false };
            var bar = new BlockBar(5);
            if (st.working) bar.Working(Signal);
            var col = Col(8, Marks.AppLogo(t.LogoKey, 36).Also(l => ((FrameworkElement)l).HorizontalAlignment = HorizontalAlignment.Left));
            col.Children.Add(Col(2, Txt(t.Name, 13, Ink, true)).Also(c => { if (AppText.VersionLine(t) is { Length: > 0 } v) c.Children.Add(Txt(v, 11, Dim)); }));
            col.Children.Add(Txt(t.Desc ?? "", 11, Dim));
            col.Children.Add(Txt(st.text, 11, st.color));
            if (!st.working && AppText.CheckLine(t, DateTime.UtcNow) is { Length: > 0 } chk) col.Children.Add(Txt(chk, 11, Dim));
            if (tradeoff != null && !st.working && t.Shown is "needs_64k" or "connected" or "installed" or "not_installed")
                col.Children.Add(Txt(tradeoff, 11, h64!.Fits && h64.TokPerSec >= 0.75 * h64.NowTokPerSec ? Dim : Gold));
            // What to do next, right on the tile.
            if (!st.working)
            {
                var acts = new FlowPanel { Spacing = 8, LineSpacing = 6 };
                if (t.Shown == "needs_64k")
                    acts.Children.Add(tooBig
                        ? Btn("Pick a smaller model", Kind.Small, () => Go("Models"))
                        : Btn("Connect again", Kind.Small, () => { selectedApps.Clear(); selectedApps.Add(t.Id); _ = ConnectApps(); }).Also(x => x.IsEnabled = !busy && ServerState == 2));
                if (t.Shown == "broken")
                {
                    acts.Children.Add(Btn("Show details", Kind.Small, () => _ = ShowAppLog(t)));
                    acts.Children.Add(Btn("Report a problem", Kind.Small, () => _ = ReportAppProblem(t, AppText.Status(t).Text)));
                }
                foreach (var x in acts.Children.OfType<Button>()) Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(x, $"{((TextBlock)x.Content).Text}: {t.Name}");
                if (acts.Children.Count > 0) col.Children.Add(acts.Also(x => x.Margin = new Thickness(0, 2, 0, 0)));
            }
            col.Children.Add(st.working ? bar : new Border { Height = 5 });
            var tile = new Border
            {
                BorderThickness = new Thickness(1), BorderBrush = B(sel ? Signal : Faint), Background = B(sel ? C(0xb9a8ff, 15) : Colors.Transparent),
                Padding = new Thickness(16), MinHeight = 170, Opacity = t.Supported ? 1 : 0.5, Child = col,
            };
            Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(tile, t.Name);
            if (t.Supported && !blocked) tile.Tapped += (_, e) =>
            {
                if (busy || InsideButton(e.OriginalSource as DependencyObject, tile)) return;
                if (!selectedApps.Remove(t.Id)) selectedApps.Add(t.Id);
                ShowSection();
            };
            if (blocked) tile.Opacity = 0.6;
            tiles.Children.Add(tile);
        }
        b.Children.Add(tiles);
        var def = new CheckBox { Content = Txt("Make it the default model in those apps"), IsChecked = makeDefault, Margin = new Thickness(0, 12, 0, 0) };
        def.Checked += (_, _) => makeDefault = true; def.Unchecked += (_, _) => makeDefault = false;
        b.Children.Add(def);
        var go = Btn(selectedApps.Count == 0 ? "Pick an app above" : $"Connect {AppText.And((status?.Targets ?? new()).Where(t => selectedApps.Contains(t.Id)).Select(t => t.Name).ToList())}  ↵", Kind.Primary, () => _ = ConnectApps());
        go.IsEnabled = selectedApps.Count > 0 && !busy && ServerState == 2;
        b.Children.Add(go.Also(x => x.Margin = new Thickness(0, 14, 0, 0)));
        if (ServerState != 2) b.Children.Add(Txt("Start your server first: connecting checks that the app gets an answer.", 12, Gold).Also(t => t.Margin = new Thickness(0, 8, 0, 0)));
        if (busy) b.Children.Add(Txt("The first answer can take a minute: coding agents send long instructions with every request.", 12, Dim).Also(t => t.Margin = new Thickness(0, 8, 0, 0)));
        if (note != "") b.Children.Add(Txt(note, 12, Dim).Also(t => t.Margin = new Thickness(0, 8, 0, 0)));
    }

    static Windows.UI.Color ToneColor(AppText.Tone t) => t switch { AppText.Tone.Ok => Ok, AppText.Tone.Gold => Gold, AppText.Tone.Bad => Bad, _ => Dim };

    /// A tap on a button inside a tile is the button's, not a tile selection.
    static bool InsideButton(DependencyObject? d, UIElement tile)
    {
        for (; d != null && d != tile; d = Microsoft.UI.Xaml.Media.VisualTreeHelper.GetParent(d))
            if (d is Microsoft.UI.Xaml.Controls.Primitives.ButtonBase) return true;
        return false;
    }

    async Task ConnectApps()
    {
        if (selectedApps.Count == 0) return;
        foreach (var id in selectedApps) appState[id] = ("Waiting…", Dim, true);
        connecting = true;
        logo?.SetBusy(true); note = ""; ShowSection();
        try { await engine.Post("integrate", new { ids = selectedApps.ToArray(), @default = makeDefault }); }
        catch (Exception ex) { connecting = false; logo?.SetBusy(false); note = ex.Message; appState.Clear(); ShowSection(); }
    }

    void OnIntegrateEvent(EngineEvent e)
    {
        if (!connecting) return; // not a connect this window started
        if (e.Step == "done")
        {
            connecting = false;
            logo?.SetBusy(false);
            note = e.State == "ok" ? "Done. Open the app and pick the model named EZLM." : e.State == "cancelled" ? "Cancelled." : "";
            if (e.State == "ok") Toast("Connected. Your model now shows up in those apps as EZLM.", Ok);
            selectedApps.Clear();
            _ = engine.Get<Status>("status").ContinueWith(t => Ui(() =>
            {
                if (t.IsCompletedSuccessfully) status = t.Result;
                foreach (var k in appState.Keys.ToList()) if (appState[k].working) appState.Remove(k);
                if (section == "Connect") ShowSection();
            }));
            return;
        }
        var id = e.Step.Replace("integrate:", "");
        appState[id] = e.State switch
        {
            "ok" => ("✓ Connected and answering", Ok, false),
            "fail" => (e.Msg ?? "Failed", Bad, false),
            _ => ((e.Msg ?? "").TrimEnd('…') + "…", Dim, true),
        };
        if (section == "Connect") ShowSection();
    }

    // MARK: Keys

    bool keyWork;

    void KeysPane(StackPanel b)
    {
        Header(b, "Keys", "Give each device or app its own key, so you can revoke one without breaking the others.");
        var c = status?.Config;
        if (c == null) return;
        var keys = c.Keys ?? new() { new ApiKey { Id = "k1", Name = "Default", Key = c.Key } };
        b.Children.Add(Hairline());
        foreach (var (k, i) in keys.Select((k, i) => (k, i)))
        {
            var g = new Grid { Padding = new Thickness(0, 10, 0, 10), ColumnSpacing = 14 };
            g.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(170) });
            g.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
            g.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
            var name = Col(2, Txt(k.Name)); if (i == 0) name.Children.Add(Txt("used by connected apps", 11, Dim));
            g.Children.Add(name);
            var v = Txt(Fmt.Masked(k.Key)); Grid.SetColumn(v, 1); g.Children.Add(v);
            var acts = Row(6);
            acts.Children.Add(CopyBtn(k.Key, $"Copy {k.Name}"));
            acts.Children.Add(Btn("Rotate", Kind.Small, () => _ = KeyAction("keys/rotate", new { id = k.Id }, $"Rotate {k.Name}?", "Anything still using the old value stops working. Apps EZLM connected are updated for you.")).Also(x => { x.IsEnabled = !keyWork; Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(x, $"Rotate {k.Name}"); }));
            var apps = (status?.Targets ?? new()).Where(t => t.Connected).Select(t => t.Name).ToList();
            // Only the first key is used by connected apps, and only when there's a second key to move them to.
            var revokeText = i == 0 && apps.Count > 0 && keys.Count > 1
                ? $"{AppText.And(apps)} {(apps.Count == 1 ? "uses" : "use")} this key; EZLM switches {(apps.Count == 1 ? "it" : "them")} to “{keys[1].Name}” for you. Anything else using it (scripts, other devices) stops working."
                : "Anything using it stops working.";
            if (keys.Count > 1) acts.Children.Add(Btn("Revoke", Kind.Small, () => _ = KeyAction("keys/revoke", new { id = k.Id }, $"Revoke {k.Name}?", revokeText)).Also(x => { x.IsEnabled = !keyWork; Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(x, $"Revoke {k.Name}"); }));
            Grid.SetColumn(acts, 2); g.Children.Add(acts);
            b.Children.Add(g); b.Children.Add(Hairline());
        }
        var box = new TextBox { PlaceholderText = "Name it, e.g. Phone or Work laptop", FontFamily = Mono, MinWidth = 320, CornerRadius = new CornerRadius(0), IsEnabled = !keyWork };
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(box, "New key name");
        var add = Btn("New key", Kind.Normal, () => _ = KeyAction("keys/add", new { name = box.Text.Trim() }, null, null));
        add.IsEnabled = !keyWork;
        box.KeyDown += (sender, a) => { if (a.Key == VirtualKey.Enter && !keyWork) { a.Handled = true; _ = KeyAction("keys/add", new { name = box.Text.Trim() }, null, null); } };
        b.Children.Add(Row(10, box, add).Also(r => r.Margin = new Thickness(0, 14, 0, 0)));
        if (keyWork) { var w = new BlockBar(5) { Margin = new Thickness(0, 10, 0, 0) }; w.Working(Signal); b.Children.Add(w); }
        if (note != "") b.Children.Add(Txt(note, 12, Dim).Also(t => t.Margin = new Thickness(0, 6, 0, 0)));

        b.Children.Add(Section("Network"));
        var lan = new CheckBox { Content = Runs(13, ("Let other devices on my network use it ", Ink), ("(phones, other computers)", Dim)), IsChecked = c.Lan, IsEnabled = !keyWork };
        lan.Click += (_, _) => _ = KeyAction("lan", new { on = lan.IsChecked == true }, null, null);
        b.Children.Add(lan);
        if (!string.IsNullOrEmpty(status?.LanUrl)) b.Children.Add(Fact("LAN URL", status!.LanUrl!).Also(f => f.Margin = new Thickness(0, 8, 0, 0)));
    }

    async Task KeyAction(string path, object body, string? confirmTitle, string? confirmText)
    {
        if (keyWork) return;
        if (confirmTitle != null && await ShowDialog(Dialog(confirmTitle, confirmText ?? "", "Continue")) != ContentDialogResult.Primary) return;
        keyWork = true; note = "Working… the server restarts so the change takes effect."; logo?.SetBusy(true); ShowSection();
        try { await engine.Post(path, body); } catch (Exception ex) { keyWork = false; logo?.SetBusy(false); note = ex.Message; ShowSection(); }
    }

    void OnRestartJobEvent(EngineEvent e)
    {
        if (!keyWork) return; // not a change this window started
        if (e.Step != "done") { note = e.Msg ?? note; if (section == "Keys") ShowSection(); return; }
        _ = engine.Get<Status>("status").ContinueWith(t => Ui(() =>
        {
            if (t.IsCompletedSuccessfully) status = t.Result;
            keyWork = false; logo?.SetBusy(false);
            note = e.State == "ok" ? "" : e.Msg ?? "";
            if (e.State == "ok") Toast(e.Job == "lan" ? (status?.Config?.Lan == true ? "Other devices on your network can use it now." : "Only this computer can use it now.") : "Done. The server is back up with the new keys.", Ok);
            if (section is "Keys" or "Overview") ShowSection();
        }));
    }

    // MARK: Logs

    TextBlock? logView;
    ScrollViewer? logScroll;
    List<string> logLines = new();
    string logSig = "";

    void LogsPane(StackPanel b)
    {
        var head = new Grid();
        head.Children.Add(Col(4, Txt("Logs", 20, Ink, true), Txt("What the model server is doing, updated live.", 12, Dim)));
        var acts = Row(8, Btn("Copy all", Kind.Small, () => Copy(string.Join("\n", logLines))), Btn("Copy details for support", Kind.Small, () => _ = CopyDiagnostics()), Btn("Report a problem", Kind.Small, () => _ = ReportProblem()));
        acts.HorizontalAlignment = HorizontalAlignment.Right; acts.VerticalAlignment = VerticalAlignment.Top; head.Children.Add(acts);
        b.Children.Add(head.Also(h => h.Margin = new Thickness(0, 0, 0, 22)));
        if (note != "") b.Children.Add(Txt(note, 12, Dim).Also(t => t.Margin = new Thickness(0, -12, 0, 12)));
        logView = new TextBlock { FontFamily = Mono, FontSize = 11, TextWrapping = TextWrapping.NoWrap, IsTextSelectionEnabled = true };
        b.Children.Add(new Border { BorderBrush = B(Rule), BorderThickness = new Thickness(1), Padding = new Thickness(14), Child = new ScrollViewer { Content = logView, HorizontalScrollBarVisibility = ScrollBarVisibility.Auto, VerticalScrollBarVisibility = ScrollBarVisibility.Disabled } });
        logSig = "";
        RenderLogs();
        _ = RefreshLogs();
    }

    async Task RefreshLogs() { try { logLines = (await engine.Get<LogLines>("logs")).Lines; RenderLogs(); } catch { } }

    void RenderLogs()
    {
        if (logView == null) return;
        var sig = $"{logLines.Count}:{logLines.LastOrDefault()}";
        if (sig == logSig) return; // unchanged: don't touch the text (keeps selection and scroll)
        logSig = sig;
        bool atBottom = logScroll == null || logScroll.VerticalOffset >= logScroll.ScrollableHeight - 40;
        logView.Inlines.Clear();
        if (logLines.Count == 0) { logView.Inlines.Add(new Run { Text = "No log output yet.", Foreground = B(Dim) }); return; }
        foreach (var l in logLines.TakeLast(300))
        {
            var c = l.Contains(" E ") || l.Contains("error", StringComparison.OrdinalIgnoreCase) ? Bad : l.Contains("print_timing") ? Ink : Dim;
            logView.Inlines.Add(new Run { Text = l + "\n", Foreground = B(c) });
        }
        if (atBottom) logScroll?.UpdateLayout();
        if (atBottom) logScroll?.ChangeView(null, logScroll.ScrollableHeight, null, true);
    }

    // MARK: events + keys

    void OnEvent(EngineEvent e)
    {
        switch (e.Job)
        {
            case "deploy": OnDeployEvent(e); break;
            case "integrate": OnIntegrateEvent(e); break;
            case "keys": case "lan": OnRestartJobEvent(e); break;
            case "update": OnUpdateEvent(e); break;
            case "launch": OnLaunchEvent(e); break;
            case "storage": OnStorageEvent(e); break;
            case "test":
                if (!testing) break; // not a test this window started
                if (e.Step == "done") { testing = false; logo?.SetBusy(false); if (e.State == "fail") note = e.Msg ?? ""; if (section == "Overview" && screen == Screen.Main) ShowSection(); }
                else if (e.TestResult is { } r) { tests.Add(r); if (section == "Overview" && screen == Screen.Main) RenderTests(); }
                break;
        }
    }

    void OnKey(object sender, KeyRoutedEventArgs e)
    {
        if (e.OriginalSource is TextBox) return;
        if (ChatKey(e)) return;
        bool inModels = screen == Screen.Main && section == "Models" && modelsList != null && !dialogOpen;
        if (screen == Screen.Choose || inModels)
        {
            var plans = VisiblePlans;
            if (plans.Count == 0) return;
            int i = plans.FindIndex(p => p.Model.Id == chosenId);
            if (e.Key == VirtualKey.Down) { chosenId = plans[Math.Min(plans.Count - 1, i + 1)].Model.Id; RefreshSelection(); e.Handled = true; }
            else if (e.Key == VirtualKey.Up) { chosenId = plans[Math.Max(0, i - 1)].Model.Id; RefreshSelection(); e.Handled = true; }
            else if (e.Key == VirtualKey.Enter && Chosen != null && !installRunning) { _ = Deploy(); e.Handled = true; }
        }
        else if (e.Key == VirtualKey.Enter && screen == Screen.Ready) { EnterMain(); e.Handled = true; }
        else if (e.Key == VirtualKey.Escape && screen == Screen.Installing && !cancelling) { _ = CancelInstall(); e.Handled = true; }
    }

    void RefreshSelection() { if (screen == Screen.Choose) UpdateChoose(); else UpdateModelsSelection(); }
}

/// 5-row pixel font drawn with half blocks, for the app marks.
static class PixelFont
{
    static readonly Dictionary<char, string[]> G = new()
    {
        ['o'] = new[] { ".##.", "#..#", "#..#", "#..#", ".##." }, ['p'] = new[] { "###.", "#..#", "###.", "#...", "#..." },
        ['e'] = new[] { "####", "#...", "###.", "#...", "####" }, ['n'] = new[] { "#..#", "##.#", "#.##", "#..#", "#..#" },
        ['c'] = new[] { ".###", "#...", "#...", "#...", ".###" }, ['d'] = new[] { "###.", "#..#", "#..#", "#..#", "###." },
        ['h'] = new[] { "#..#", "#..#", "####", "#..#", "#..#" }, ['r'] = new[] { "###.", "#..#", "###.", "#.#.", "#..#" },
        ['m'] = new[] { "#...#", "##.##", "#.#.#", "#...#", "#...#" }, ['s'] = new[] { ".###", "#...", ".##.", "...#", "###." },
    };

    public static string[] Lines(string word)
    {
        var rows = Enumerable.Range(0, 6).Select(r => string.Concat(word.Select(ch => (r < 5 ? G[ch][r] : new string('.', G[ch][0].Length)) + "."))).ToArray();
        var outp = new string[3];
        for (int r = 0; r < 6; r += 2)
            outp[r / 2] = new string(rows[r].Select((c, i) => (c == '#', rows[r + 1][i] == '#') switch { (true, true) => '█', (true, false) => '▀', (false, true) => '▄', _ => ' ' }).ToArray());
        return outp;
    }
}

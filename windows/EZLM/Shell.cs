using Microsoft.UI;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Windowing;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media;
using Windows.System;
using static EZLM.Ui;

namespace EZLM;

/// The EZLM window: a first-run setup wizard, then a sidebar app.
///
/// Screens are built once and then updated in place: selection changes only
/// restyle cards, progress events only change texts and bars, and polling
/// only touches the numbers. Rebuilding whole screens on every event is what
/// made earlier versions feel slow and flickery.
public sealed partial class Shell : Window
{
    readonly Engine engine = new();
    readonly Microsoft.UI.Dispatching.DispatcherQueue dq;
    readonly CancellationTokenSource quit = new();
    readonly Grid root = new() { Background = B(Black) };
    readonly ContentControl host = new() { HorizontalContentAlignment = HorizontalAlignment.Stretch, VerticalContentAlignment = VerticalAlignment.Stretch };
    readonly TextBlock foot = Txt("", 11, Faint);

    // State
    enum Screen { Booting, Scanning, Choose, Installing, Ready, Failed, Main }
    Screen screen = Screen.Booting;
    string pref = "balanced";
    ScanResult? scan;
    string? chosenId;
    Status? status;
    Live? live;
    List<InstalledModel> installed = new();
    string note = "";
    string reply = "";
    string section = "Overview";
    Logo? logo;

    public Shell()
    {
        dq = Microsoft.UI.Dispatching.DispatcherQueue.GetForCurrentThread();
        Title = "EZLM";
        ExtendsContentIntoTitleBar = true;
        FitToScreen();
        var icon = System.IO.Path.Combine(AppContext.BaseDirectory, "Assets", "EZLM.ico");
        if (File.Exists(icon)) AppWindow.SetIcon(icon);
        if (AppWindow.TitleBar is { } tb)
        {
            tb.ButtonBackgroundColor = Colors.Transparent;
            tb.ButtonInactiveBackgroundColor = Colors.Transparent;
            tb.ButtonForegroundColor = Dim;
            tb.ButtonHoverBackgroundColor = C(0x1a1a1a);
        }

        root.RowDefinitions.Add(new RowDefinition { Height = new GridLength(32) });
        root.RowDefinitions.Add(new RowDefinition { Height = new GridLength(1, GridUnitType.Star) });
        root.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        var drag = new Grid { Background = B(Black) };
        root.Children.Add(drag);
        SetTitleBar(drag);
        Grid.SetRow(host, 1); root.Children.Add(host);
        foot.HorizontalAlignment = HorizontalAlignment.Center; foot.Margin = new Thickness(0, 6, 0, 10);
        foot.Text = Engine.Demo ? "Demo: nothing is installed. Data is from an RTX 4070 SUPER desktop." : "Everything runs on this computer. Nothing you type is sent anywhere else.";
        Grid.SetRow(foot, 2); root.Children.Add(foot);
        toastText = Txt("", 13);
        toast = new Border
        {
            Child = toastText, Padding = new Thickness(16, 10, 16, 10), Background = B(C(0x141414)), BorderBrush = B(Faint), BorderThickness = new Thickness(1),
            HorizontalAlignment = HorizontalAlignment.Center, VerticalAlignment = VerticalAlignment.Bottom, Margin = new Thickness(0, 0, 0, 18),
            Visibility = Visibility.Collapsed, MaxWidth = 720,
        };
        Grid.SetRow(toast, 1); root.Children.Add(toast);
        root.PreviewKeyDown += OnKey; // tunneling: the sidebar would otherwise eat arrow keys
        // App logos are decoded for the display's scale; moving to a screen with another scale redraws them.
        root.Loaded += (_, _) =>
        {
            Marks.Scale = root.XamlRoot.RasterizationScale;
            root.XamlRoot.Changed += (x, _) =>
            {
                if (Math.Abs(Marks.Scale - x.RasterizationScale) < 0.01) return;
                Marks.Scale = x.RasterizationScale;
                if (screen == Screen.Main && section != "Chat") ShowSection();
            };
        };
        Content = root;
        engine.Exited += () => Ui(EngineDied);
        Closed += (_, _) => { quit.Cancel(); engine.Dispose(); };
        AppWindow.Closing += OnWindowClosing;
        // Back in the window: a finished job's red taskbar bar has done its job.
        Activated += (_, e) => { if (e.WindowActivationState != WindowActivationState.Deactivated && !installRunning && !storageBusy) Taskbar.Clear(Hwnd); };
        _ = Boot();
    }

    Border? toast;
    TextBlock? toastText;
    int toastId;

    /// A short confirmation at the bottom of the window that fades by itself.
    public async void Toast(string text, Windows.UI.Color? color = null)
    {
        if (toast == null || toastText == null) return;
        var id = ++toastId;
        toastText.Text = text; toastText.Foreground = B(color ?? Ink);
        toast.BorderBrush = B(color is { } c ? C((uint)(c.R << 16 | c.G << 8 | c.B), 140) : Faint);
        toast.Opacity = 1; toast.Visibility = Visibility.Visible;
        await Task.Delay(4200);
        for (int i = 10; i >= 0 && id == toastId; i--) { toast.Opacity = i / 10.0; await Task.Delay(30); }
        if (id == toastId) toast.Visibility = Visibility.Collapsed;
    }

    [System.Runtime.InteropServices.DllImport("user32.dll")]
    static extern uint GetDpiForWindow(IntPtr hwnd);

    /// AppWindow sizes are physical pixels: scale for the display's DPI (a
    /// 150% laptop would otherwise get a cramped window) and never exceed the
    /// screen's work area (a 1366x768 laptop at 125% is only ~614 px tall).
    void FitToScreen()
    {
        try
        {
            var hwnd = WinRT.Interop.WindowNative.GetWindowHandle(this);
            double scale = Math.Max(1, GetDpiForWindow(hwnd) / 96.0);
            var wa = DisplayArea.GetFromWindowId(AppWindow.Id, DisplayAreaFallback.Primary).WorkArea;
            int w = (int)Math.Min(1180 * scale, wa.Width * 0.94), h = (int)Math.Min(800 * scale, wa.Height * 0.94);
            AppWindow.MoveAndResize(new Windows.Graphics.RectInt32(wa.X + (wa.Width - w) / 2, wa.Y + (wa.Height - h) / 2, w, h));
            if (AppWindow.Presenter is OverlappedPresenter op)
            {
                op.PreferredMinimumWidth = (int)Math.Min(900 * scale, wa.Width * 0.9);
                op.PreferredMinimumHeight = (int)Math.Min(600 * scale, wa.Height * 0.9);
            }
        }
        catch (Exception ex) { Log(ex); AppWindow.Resize(new Windows.Graphics.SizeInt32(1180, 800)); }
    }

    IntPtr Hwnd => WinRT.Interop.WindowNative.GetWindowHandle(this);

    void Ui(Action a) => dq.TryEnqueue(() => { try { a(); } catch (Exception ex) { Log(ex); } });

    // MARK: boot

    bool listening;

    async Task Boot()
    {
        screen = Screen.Booting;
        host.Content = Wizard(Col(10, Row(10, new Spinner(), Txt("Starting EZLM"))));
        try
        {
            await engine.StartAsync();
            // One event listener for the window's lifetime; it reconnects on its own after a restart.
            if (!listening) { listening = true; _ = engine.ListenAsync(e => Ui(() => OnEvent(e)), quit.Token); }
            status = await engine.Get<Status>("status");
            // Opening EZLM means you want your AI on: start it unless you stopped it yourself.
            if (status.Deployed && !status.Healthy && !status.Starting && !status.UserStopped && !Engine.Demo)
            {
                try { await engine.Post("start"); status.Starting = true; } catch (Exception ex) { Log(ex); }
            }
            if (status.Deployed) EnterMain();
            else await RunScan();
        }
        catch (Exception ex) { Log(ex); ShowFailed(ex.Message.Split('\n')[0].Trim(), () => _ = Boot()); }
    }

    void ShowFailed(string message, Action retry)
    {
        screen = Screen.Failed;
        logo?.SetBusy(false);
        host.Content = Wizard(Col(14,
            Txt("Something went wrong", 18, Ink, true),
            Txt(message, 13, Bad),
            Row(12, Btn("Try again", Kind.Primary, retry), Btn("Report this problem", Kind.Normal, () => _ = ReportProblem(message)), Btn("Copy details for support", Kind.Small, () => _ = CopyDiagnostics()))));
    }

    bool engineGone;
    void EngineDied()
    {
        if (quit.IsCancellationRequested || engineGone) return;
        engineGone = true;
        Log(new Exception("engine exited unexpectedly"));
        try { allowDialogClose = true; installDialog?.Hide(); } catch { }
        installRunning = false;
        ShowFailed("EZLM's background engine stopped unexpectedly. Your model server keeps running; Try again reconnects to it.", () => { engineGone = false; _ = Boot(); });
    }

    /// Full error details for debugging: %LOCALAPPDATA%\EZLM\app.log
    public static void Log(Exception ex)
    {
        try
        {
            var path = System.IO.Path.Combine(Engine.DataDir, "app.log");
            if (File.Exists(path) && new FileInfo(path).Length > 2_000_000) File.Delete(path);
            File.AppendAllText(path, $"{DateTime.Now:O} {ex}\n\n");
        }
        catch { }
    }

    /// Everything someone helping a beta tester needs, on the clipboard.
    async Task CopyDiagnostics()
    {
        var sb = new System.Text.StringBuilder();
        sb.AppendLine($"EZLM {typeof(Shell).Assembly.GetName().Version} on {Environment.OSVersion} ({System.Runtime.InteropServices.RuntimeInformation.OSArchitecture})");
        try { sb.AppendLine("status: " + System.Text.Json.JsonSerializer.Serialize(await engine.Get<System.Text.Json.JsonElement>("status"))); } catch (Exception ex) { sb.AppendLine("status: " + ex.Message); }
        if (scan != null) sb.AppendLine("hardware: " + System.Text.Json.JsonSerializer.Serialize(scan.Hw, Engine.Json));
        if (live != null) sb.AppendLine("live: " + System.Text.Json.JsonSerializer.Serialize(live, Engine.Json));
        try { sb.AppendLine("server log:\n" + string.Join("\n", (await engine.Get<LogLines>("logs")).Lines.TakeLast(60))); } catch { }
        foreach (var f in new[] { "engine.log", "app.log" })
        {
            var p = System.IO.Path.Combine(Engine.DataDir, f);
            try { if (File.Exists(p)) sb.AppendLine($"{f}:\n" + string.Join("\n", File.ReadLines(p).TakeLast(60))); } catch { }
        }
        // Never hand out API keys: anything shaped like an EZLM key is hidden.
        var text = System.Text.RegularExpressions.Regex.Replace(sb.ToString(), @"ezlm-[A-Za-z0-9]{6,}", m => m.Value[..9] + "…(hidden)");
        Copy(text);
        Toast("Copied (your keys are hidden). Paste it in a message to whoever is helping you.", Ok);
    }

    // MARK: wizard chrome

    readonly string[] stepNames = { "Scan", "Choose", "Install", "Ready" };

    int StepIndex => screen switch { Screen.Choose => 1, Screen.Installing => 2, Screen.Ready => 3, _ => 0 };

    UIElement Wizard(UIElement content)
    {
        var g = new Grid { Padding = new Thickness(40, 8, 40, 0) };
        g.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        g.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        g.RowDefinitions.Add(new RowDefinition { Height = new GridLength(1, GridUnitType.Star) });

        var header = new Grid();
        header.Children.Add(MiniLogo());
        var steps = Row(18);
        steps.HorizontalAlignment = HorizontalAlignment.Right;
        for (int i = 0; i < 4; i++)
        {
            bool done = i < StepIndex, cur = i == StepIndex;
            var num = new Border
            {
                Width = 22, Height = 22, BorderThickness = new Thickness(1),
                BorderBrush = B(cur ? Ink : done ? C(0x8fe3b5, 128) : Faint), Background = B(cur ? Ink : Colors.Transparent),
                Child = new TextBlock { Text = done ? "✓" : $"{i + 1}", FontFamily = Mono, FontSize = 11, FontWeight = Microsoft.UI.Text.FontWeights.Bold, Foreground = B(cur ? Black : done ? Ok : Dim), HorizontalAlignment = HorizontalAlignment.Center, VerticalAlignment = VerticalAlignment.Center },
            };
            var name = Txt(stepNames[i], 12, cur ? Ink : Dim, cur); name.VerticalAlignment = VerticalAlignment.Center;
            steps.Children.Add(Row(8, num, name));
            if (i < 3) steps.Children.Add(new Border { Width = 26, Height = 1, Background = B(done ? C(0x8fe3b5, 128) : Rule), VerticalAlignment = VerticalAlignment.Center });
        }
        header.Children.Add(steps);
        g.Children.Add(header);
        var line = Hairline(); line.Margin = new Thickness(-40, 16, -40, 0); Grid.SetRow(line, 1); g.Children.Add(line);
        if (content is FrameworkElement fe && fe.HorizontalAlignment == HorizontalAlignment.Stretch && screen != Screen.Choose) { fe.MaxWidth = 620; fe.HorizontalAlignment = HorizontalAlignment.Left; }
        Grid.SetRow((FrameworkElement)content, 2); g.Children.Add(content);
        return g;
    }

    static UIElement MiniLogo() => Runs(18, ("EZ", Dim), ("LM", Ink)).Also(t => { t.FontWeight = Microsoft.UI.Text.FontWeights.Black; t.VerticalAlignment = VerticalAlignment.Center; });

    // MARK: scan

    readonly (string id, string label)[] scanLines = { ("cpu", "Processor"), ("mem", "System memory"), ("gpu", "Graphics"), ("disk", "Disk space"), ("fit", "Comparing models against your memory") };

    async Task RunScan()
    {
        screen = Screen.Scanning;
        logo = new Logo();
        logo.SetBusy(true);
        var rows = new Dictionary<string, (Border mark, TextBlock sub)>();
        var list = Col(4);
        foreach (var (id, label) in scanLines)
        {
            var mark = new Border { Width = 16, Child = new Spinner() };
            var sub = Txt("", 12, Dim);
            list.Children.Add(Row(10, mark, Col(2, Txt(label), sub)));
            rows[id] = (mark, sub);
        }
        var content = Col(0, logo, Txt("Your own AI model, running on this computer.", 13, Dim).Also(t => t.Margin = new Thickness(0, 16, 0, 28)), list);
        content.VerticalAlignment = VerticalAlignment.Center;
        host.Content = Wizard(content);
        try
        {
            var task = engine.Post<ScanResult>("scan", new { pref });
            await Task.Delay(700);
            var r = await task;
            var hw = r.Hw;
            var vals = new Dictionary<string, string>
            {
                ["cpu"] = $"{hw.CpuName}, {hw.Cores} threads",
                ["mem"] = $"{Fmt.Gb(hw.RamAvail)} free of {Fmt.Gb(hw.RamTotal)}",
                ["gpu"] = hw.Unified ? $"Shares system memory, up to {Fmt.Gb(hw.GpuUsable)} for AI" : (hw.Gpus ?? new()).Count == 0 ? "No graphics card found, the processor will run it" : string.Join("; ", hw.Gpus!.Select(g => $"{g.Name}, {Fmt.Gb(g.MemFree)} free of {Fmt.Gb(g.MemTotal)}")),
                ["disk"] = $"{Fmt.Gb(hw.DiskFree)} free",
                ["fit"] = r.Recommended != null ? $"{r.Plans?.Count ?? 0} ways to run a model here" : "Nothing fits right now",
            };
            foreach (var (id, _) in scanLines)
            {
                await Task.Delay(160);
                bool ok = !(id == "fit" && r.Recommended == null);
                rows[id].mark.Child = Txt(ok ? "✓" : "✗", 13, ok ? Ok : Bad);
                rows[id].sub.Text = vals[id];
            }
            await Task.Delay(300);
            scan = r;
            chosenId = r.Recommended?.Model.Id;
            logo.SetBusy(false);
            screen = Screen.Choose;
            chooseBuiltFor = null;
            ShowChoose();
        }
        catch (Exception ex) { Log(ex); ShowFailed(ex.Message, () => _ = RunScan()); }
    }

    // In the app, only what's visibly selected counts (never a silent fallback
    // to "Recommended"); the wizard starts with the recommended one selected.
    Plan? Chosen => scan?.Plans?.FirstOrDefault(p => p.Model.Id == chosenId) ?? (screen == Screen.Main ? null : scan?.Recommended);
    List<Plan> VisiblePlans => (scan?.Plans ?? new()).Take(14).ToList();

    // MARK: choose

    PlanList? chooseList;
    StackPanel? chooseInfo;
    Button? chooseGo;
    TextBlock? chooseNote;
    UIElement? chooseRoot;
    ScanResult? chooseBuiltFor;
    string? typedName;

    void ShowChoose()
    {
        if (scan == null) return;
        if (chooseBuiltFor == scan && chooseRoot != null && host.Content == chooseRoot) { UpdateChoose(); return; }
        var r = scan;
        var grid = new Grid { Margin = new Thickness(0, 24, 0, 0) };
        grid.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(420) });
        grid.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1) });
        grid.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });

        // Left: the pick and where it lands in memory.
        var left = new Grid { Margin = new Thickness(0, 0, 32, 0) };
        left.RowDefinitions.Add(new RowDefinition { Height = new GridLength(1, GridUnitType.Star) });
        left.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        chooseInfo = Col(0);
        left.Children.Add(new ScrollViewer { Content = chooseInfo, VerticalScrollBarVisibility = ScrollBarVisibility.Auto });
        chooseGo = Btn("Set it up  ↵", Kind.Primary, () => _ = Deploy());
        chooseNote = Txt("", 12, Bad).Also(t => { t.VerticalAlignment = VerticalAlignment.Center; t.MaxWidth = 380; });
        var actions = Col(8, Row(12, chooseGo, Btn("Scan again", Kind.Normal, () => _ = RunScan()), Btn("Free up memory", Kind.Normal, () => _ = ModelMode(() => _ = RunScan()))), chooseNote);
        actions.Margin = new Thickness(0, 12, 0, 16);
        Grid.SetRow(actions, 1); left.Children.Add(actions);
        grid.Children.Add(left);

        var divider = Hairline(true); Grid.SetColumn(divider, 1); grid.Children.Add(divider);

        // Right: preference, the other options, and what's in the way.
        var right = new Grid { Margin = new Thickness(28, 0, 0, 0) };
        right.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        right.RowDefinitions.Add(new RowDefinition { Height = new GridLength(1, GridUnitType.Star) });
        right.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        right.Children.Add(Col(6, PrefPicker(), Txt("Other ways to run a model here   ↑↓ to choose, ↵ to set up", 13, Dim).Also(t => t.Margin = new Thickness(0, 10, 0, 4))));
        chooseList = new PlanList(VisiblePlans, r.Recommended?.Model.Id, status?.Deployed == true ? status.Config?.ModelId : null, r.Hw, id => { chosenId = id; UpdateChoose(); });
        var sv = new ScrollViewer { Content = chooseList.Panel, VerticalScrollBarVisibility = ScrollBarVisibility.Auto };
        Grid.SetRow(sv, 1); right.Children.Add(sv);
        var procs = r.Hw.GpuProcs ?? new();
        if (procs.Count > 0)
        {
            var ps = Col(4, Txt("Using your graphics card", 13, Dim).Also(t => t.Margin = new Thickness(0, 10, 0, 2)));
            foreach (var pr in procs.Take(2)) ps.Children.Add(ProcRow(pr));
            Grid.SetRow(ps, 2); right.Children.Add(ps);
        }
        Grid.SetColumn(right, 2); grid.Children.Add(right);
        chooseRoot = Wizard(grid);
        host.Content = chooseRoot;
        chooseBuiltFor = scan;
        UpdateChoose();
    }

    void UpdateChoose()
    {
        if (scan == null || chooseInfo == null) return;
        var r = scan; var p = Chosen;
        chooseList?.Select(p?.Model.Id);
        chooseInfo.Children.Clear();
        if (p != null)
        {
            chooseInfo.Children.Add(Txt($"Your {r.Hw.MachineWord} can run", 13, Dim));
            var name = Txt(p.Model.Name, 24, Ink, true);
            name.MaxWidth = 360; // wrap long names instead of clipping
            chooseInfo.Children.Add(Row(12, Marks.Logo(p.Model.Vendor, 30), name).Also(s => s.Margin = new Thickness(0, 6, 0, 6)));
            if (typedName != p.Model.Name) { typedName = p.Model.Name; Typer.Type(name, p.Model.Name, 22); } // only when the pick changes
            chooseInfo.Children.Add(Marks.Origin(p.Model.Country).Also(o => ((FrameworkElement)o).Margin = new Thickness(0, 0, 0, 10)));
            chooseInfo.Children.Add(Fact("File", p.QuantLine));
            chooseInfo.Children.Add(Fact("Download", p.Downloaded ? "Already on this computer" : p.Partial > 0 ? $"{Fmt.Gb(p.Download)} left ({Fmt.Gb(p.Partial)} already here)" : Fmt.Gb(p.Download > 0 ? p.Download : p.Quant.Bytes)));
            if (!p.Downloaded && !string.IsNullOrEmpty(r.Hw.ModelsDir))
            {
                var where = new Grid { Margin = new Thickness(0, 1.5, 0, 1.5) };
                where.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(110) });
                where.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
                where.Children.Add(Txt("Saves to", 13, Dim));
                var val = Row(10, Txt(r.Hw.ModelsDir, 13, Ink).Also(t => { t.TextWrapping = TextWrapping.Wrap; t.MaxWidth = 210; }), Btn("Change", Kind.Small, () => _ = StorageDialog()).Also(x => x.VerticalAlignment = VerticalAlignment.Top));
                Grid.SetColumn(val, 1); where.Children.Add(val);
                chooseInfo.Children.Add(where);
            }
            chooseInfo.Children.Add(Fact("Speed", $"{p.SpeedWord}, about {p.Words} words/s"));
            chooseInfo.Children.Add(Fact("Context", $"{p.Ctx / 1024}k tokens"));
            chooseInfo.Children.Add(Fact("Tools", p.Model.ToolCalling ? "Works with coding agents" : "Chat only"));
            if (!p.DiskOk && !p.Downloaded)
                chooseInfo.Children.Add(Runs(11, ($"Needs about {Fmt.Gb((p.Download > 0 ? p.Download : p.Quant.Bytes) + (2UL << 30))} of disk, {Fmt.Gb(r.Hw.DiskFree)} is free. ", Gold), ("You can still set it up; make room before the download finishes.", Dim)).Also(t => t.Margin = new Thickness(0, 8, 0, 0)));
            chooseInfo.Children.Add(new Border { Height = 14 });
            foreach (var row in MemoryRows(r.Hw, p)) chooseInfo.Children.Add(row);
        }
        else chooseInfo.Children.Add(Txt("No model fits in the memory that's free right now. Quit an app on the right, then scan again.", 15, Bad, true));
        if (r.Unlock is { } u)
            chooseInfo.Children.Add(Runs(12, (r.UnlockGpu ? "Your graphics card is busy. " : "", Dim), ($"Free about {Math.Ceiling(r.UnlockFreeGb)} GB to run ", Dim), (u.Model.Name, Ink), ($" at ~{u.Words} words/s.", Dim)).Also(t => t.Margin = new Thickness(0, 6, 0, 0)));
        if (chooseGo != null)
        {
            ((TextBlock)chooseGo.Content).Text = p?.Downloaded == true ? "Set it up (no download)  ↵" : p?.Partial > 0 ? "Resume and set it up  ↵" : "Set it up  ↵";
            chooseGo.IsEnabled = p != null && !installRunning;
        }
        if (chooseNote != null) chooseNote.Text = note;
    }

    bool prefBusy;

    UIElement PrefPicker()
    {
        var row = Row(0);
        var blurbText = Txt("", 11, Dim);
        void Paint()
        {
            int k = 0;
            foreach (var (id, _) in Prefs)
            {
                var t = (Border)row.Children[k++];
                bool on = id == pref;
                t.Background = B(on ? Ink : Colors.Transparent);
                ((TextBlock)t.Child).Foreground = B(on ? Black : Dim);
                ((TextBlock)t.Child).FontWeight = on ? Microsoft.UI.Text.FontWeights.Bold : Microsoft.UI.Text.FontWeights.Normal;
            }
            blurbText.Text = prefBusy ? "Comparing…" : pref switch { "smartest" => "Most capable model that fits, even if slow", "fastest" => "Snappy replies first", _ => "Smart, at a comfortable speed" };
        }
        foreach (var (id, title) in Prefs)
        {
            var t = new Border { Padding = new Thickness(14, 5, 14, 5), Child = Txt(title, 12, Dim), Background = B(Colors.Transparent) };
            Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(t, title);
            t.Tapped += async (_, _) =>
            {
                if (pref == id || prefBusy) return;
                pref = id; prefBusy = true; Paint();
                await RefreshScan();
                prefBusy = false;
                chosenId = scan?.Recommended?.Model.Id;
                if (screen == Screen.Choose) ShowChoose(); else ShowSection();
            };
            row.Children.Add(t);
        }
        Paint();
        return Col(6, new Border { BorderBrush = B(Faint), BorderThickness = new Thickness(1), Child = row, HorizontalAlignment = HorizontalAlignment.Left }, blurbText);
    }

    static readonly (string id, string title)[] Prefs = { ("balanced", "Balanced"), ("smartest", "Smartest"), ("fastest", "Fastest") };

    IEnumerable<UIElement> MemoryRows(Hardware hw, Plan? p)
    {
        if (hw.Unified) { yield return MemRow("Memory, shared with graphics", hw.RamTotal, hw.RamTotal - hw.RamAvail, p?.GpuNeed); yield break; }
        if ((hw.Gpus ?? new()).Count > 0) yield return MemRow("Graphics memory", hw.GpuTotal, hw.GpuTotal - hw.GpuFree, p == null ? null : p.Mode == "cpu" ? 0 : p.GpuNeed);
        yield return MemRow("System memory", hw.RamTotal, hw.RamTotal - hw.RamAvail, p?.RamNeed);
    }

    UIElement MemRow(string label, ulong total, ulong used, ulong? need)
    {
        var free = total > used ? total - used : 0;
        var n = Math.Min(need ?? 0, free);
        var head = new Grid();
        head.Children.Add(Txt(label, 13, Dim));
        var right = need is > 0 ? Runs(12, (Fmt.Gb(n), Ink), ($" for the model, {Fmt.Gb(free - n)} left", Dim)) : need != null ? Runs(12, (Fmt.Gb(free) + " free", Ink), ($" of {Fmt.Gb(total)}, not used", Dim)) : Runs(12, (Fmt.Gb(free) + " free", Ink), ($" of {Fmt.Gb(total)}", Dim));
        right.HorizontalAlignment = HorizontalAlignment.Right; head.Children.Add(right);
        var bar = new BlockBar(18);
        bar.Set((double)used / Math.Max(total, 1), (double)n / Math.Max(total, 1));
        var legend = Row(16, Legend(Dim, $"Apps {Fmt.Gb(used)}"));
        if (n > 0) legend.Children.Add(Legend(Signal, $"Model {Fmt.Gb(n)}"));
        legend.Children.Add(Legend(Rule, $"Free {Fmt.Gb(free - n)}"));
        return Col(6, head, bar, legend).Also(s => s.Margin = new Thickness(0, 0, 0, 16));
    }

    static UIElement Legend(Windows.UI.Color c, string text) => Row(6, new Border { Width = 9, Height = 9, Background = B(c), VerticalAlignment = VerticalAlignment.Center }, Txt(text, 11, Dim));

    UIElement ProcRow(Proc p)
    {
        var g = new Grid();
        g.Children.Add(Txt(p.Name, 12));
        var r = Row(10, Txt(Fmt.Gb(p.Mem), 11, Dim).Also(t => t.VerticalAlignment = VerticalAlignment.Center));
        if (p.Killable)
            r.Children.Add(Btn("Quit", Kind.Small, async () =>
            {
                var d = Dialog($"Quit {p.Name}?", "Only this app is closed. Unsaved work in it may be lost.", "Quit");
                if (await ShowDialog(d) == ContentDialogResult.Primary) { try { await engine.Post("kill", new { pid = p.Pid }); } catch (Exception ex) { note = ex.Message; } await RunScan(); }
            }));
        r.HorizontalAlignment = HorizontalAlignment.Right; g.Children.Add(r);
        return g;
    }

    async Task RefreshScan()
    {
        try { scan = await engine.Post<ScanResult>("scan", new { pref }); } catch (Exception ex) { Log(ex); }
    }

    // Only one ContentDialog may be open at a time in WinUI.
    bool dialogOpen;
    ContentDialog Dialog(string title, string text, string primary) =>
        new() { Title = title, Content = Txt(text, 13), PrimaryButtonText = primary, CloseButtonText = "Cancel", DefaultButton = ContentDialogButton.Close, XamlRoot = root.XamlRoot, RequestedTheme = ElementTheme.Dark };

    async Task<ContentDialogResult> ShowDialog(ContentDialog d)
    {
        if (dialogOpen) return ContentDialogResult.None;
        dialogOpen = true;
        try { return await d.ShowAsync(); }
        catch (Exception ex) { Log(ex); return ContentDialogResult.None; }
        finally { dialogOpen = false; }
    }

    [System.Runtime.InteropServices.DllImport("user32.dll", CharSet = System.Runtime.InteropServices.CharSet.Unicode)]
    static extern int MessageBoxW(IntPtr hwnd, string text, string caption, uint type);

    bool closeOk, askingClose;
    Action? escCloseQuestion;

    /// Closing the window stops whatever EZLM is in the middle of: ask first,
    /// so one stray click on the X doesn't throw away a long download.
    async void OnWindowClosing(AppWindow sender, AppWindowClosingEventArgs e)
    {
        var what = installRunning ? $"EZLM is still setting up {fillName}. If you close it now, that stops. What's already downloaded is kept, and it picks up where it stopped next time."
            : storageBusy ? "EZLM is moving your models. If you close it now, the move stops and the files not moved yet stay where they are."
            : updateBusy ? "EZLM is downloading its update. If you close it now, the update stops; you can start it again in Settings."
            : chatCts != null ? "Your model is still writing an answer in Chat. If you close EZLM now, it stops there (what's written so far is kept)."
            : null;
        if (closeOk || what == null) return;
        e.Cancel = true;
        if (askingClose) return;
        askingClose = true;
        try
        {
            if (installDialog is { } d)
            {
                // Only one dialog can be open at a time: ask inside the one that's showing.
                var keep = d.Content;
                var done = new TaskCompletionSource<bool>();
                d.Closed += (_, _) => done.TrySetResult(false); // the job ended and took the dialog with it
                d.Content = new Border
                {
                    Width = 560,
                    Child = Col(14, Txt("Close EZLM?", 20, Ink, bold: true), Txt(what, 13, Dim),
                        Row(10, Btn("Keep going", Kind.Primary, () => done.TrySetResult(false)), Btn("Close EZLM", Kind.Normal, () => done.TrySetResult(true)))),
                };
                escCloseQuestion = () => done.TrySetResult(false); // Esc here means "never mind", not "cancel the switch"
                try { if (await done.Task) CloseNow(); else if (installDialog == d) d.Content = keep; }
                finally { escCloseQuestion = null; }
            }
            else if (dialogOpen)
            {
                // Some other dialog is up; a plain Windows question still works on top of it.
                if (MessageBoxW(WinRT.Interop.WindowNative.GetWindowHandle(this), what + "\n\nClose EZLM anyway?", "EZLM", 0x4 | 0x30 | 0x100) == 6) CloseNow();
            }
            else
            {
                var c = new ContentDialog
                {
                    Title = "Close EZLM?", Content = Txt(what, 13),
                    PrimaryButtonText = "Keep going", SecondaryButtonText = "Close EZLM", DefaultButton = ContentDialogButton.Primary,
                    XamlRoot = root.XamlRoot, RequestedTheme = ElementTheme.Dark,
                };
                if (await ShowDialog(c) == ContentDialogResult.Secondary) CloseNow();
            }
        }
        catch (Exception ex) { Log(ex); }
        finally { askingClose = false; }
    }

    void CloseNow() { if (chatCts != null) SaveChat(); closeOk = true; Close(); }

    // MARK: main app

    NavigationView? nav;
    ContentControl? pane;
    TextBlock? sideStatus, sideModel, sideSpeed;
    Border? sideDot;

    void EnterMain()
    {
        screen = Screen.Main;
        ShowMain();
        _ = Poll();
        // Look for a newer EZLM once per launch; the banner appears if there is one.
        _ = CheckUpdate().ContinueWith(_ => Ui(() => { if (updateInfo?.Available == true && section is "Overview" or "Settings" && !dialogOpen) ShowSection(); }));
    }

    void ShowMain()
    {
        nav = new NavigationView
        {
            PaneDisplayMode = NavigationViewPaneDisplayMode.Left, IsBackButtonVisible = NavigationViewBackButtonVisible.Collapsed,
            IsSettingsVisible = false, OpenPaneLength = 230, IsPaneToggleButtonVisible = false, RequestedTheme = ElementTheme.Dark,
        };
        nav.Resources["NavigationViewExpandedPaneBackground"] = B(C(0x0b0b0b));
        nav.Resources["NavigationViewDefaultPaneBackground"] = B(C(0x0b0b0b));
        nav.Resources["NavigationViewContentBackground"] = B(Black);
        sideDot = new Border { Width = 9, Height = 9, CornerRadius = new CornerRadius(5), VerticalAlignment = VerticalAlignment.Center };
        sideStatus = Txt("", 12, Ink, true); sideSpeed = Txt("", 11, Signal); sideModel = Txt("", 11, Dim, false, false);
        nav.PaneHeader = Col(14, MiniLogo().Also(l => ((FrameworkElement)l).Margin = new Thickness(16, 10, 0, 0)),
            new Border { Margin = new Thickness(12, 0, 12, 6), Padding = new Thickness(12), BorderBrush = B(Rule), BorderThickness = new Thickness(1), Child = Col(6, Row(8, sideDot, sideStatus, sideSpeed), sideModel) });
        foreach (var (name, glyph) in new[] { ("Overview", ""), ("Chat", "\uE8BD"), ("Models", ""), ("System", ""), ("Connect", ""), ("Keys", ""), ("Logs", ""), ("Settings", "") })
        {
            var item = new NavigationViewItem { Content = new TextBlock { Text = name, FontFamily = Mono, FontSize = 13 }, Icon = new FontIcon { Glyph = glyph }, Tag = name };
            Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(item, name);
            nav.MenuItems.Add(item);
        }
        nav.PaneFooter = Btn("Report a problem", Kind.Small, () => _ = ReportProblem()).Also(x => { x.Margin = new Thickness(16, 0, 16, 16); x.HorizontalAlignment = HorizontalAlignment.Left; });
        nav.SelectionChanged += (_, a) => { if (a.SelectedItem is NavigationViewItem i && (string)i.Tag != shownSection) { section = (string)i.Tag; note = ""; ShowSection(); } };
        pane = new ContentControl { HorizontalContentAlignment = HorizontalAlignment.Stretch, VerticalContentAlignment = VerticalAlignment.Stretch };
        nav.Content = pane;
        // NavigationView rejects SelectedItem until it's in the tree.
        nav.Loaded += (_, _) => SyncNav();
        host.Content = nav;
        UpdateSidebar();
        ShowSection();
    }

    void SyncNav()
    {
        try { if (nav?.IsLoaded == true) nav.SelectedItem = nav.MenuItems.OfType<NavigationViewItem>().First(i => (string)i.Tag == section); } catch (Exception ex) { Log(ex); }
    }

    int ServerState => (live?.Healthy ?? status?.Healthy ?? false) ? 2 : (live?.Starting ?? status?.Starting ?? false) ? 1 : 0;

    void UpdateSidebar()
    {
        if (sideDot == null) return;
        var st = ServerState;
        sideDot.Background = B(st == 2 ? Ok : st == 1 ? Gold : Bad);
        sideStatus!.Text = st == 2 ? "Live" : st == 1 ? "Loading model…" : status?.Deployed == true ? "Stopped" : "Not set up";
        sideSpeed!.Text = live is { Phase: "writing" } l ? $"{(int)l.TokPerSec} tok/s" : live is { Phase: "reading" } ? "reading" : "";
        sideModel!.Text = status?.Config?.ModelName ?? "No model";
        if (section == "Chat") UpdateChatInput();
    }

    string? shownSection;

    void ShowSection()
    {
        if (pane == null) return;
        shownSection = section;
        var body = Col(0);
        body.Padding = new Thickness(34, 30, 34, 30);
        body.MaxWidth = 860; body.HorizontalAlignment = HorizontalAlignment.Left;
        ovMem = null; ovSpeed = null; logView = null; modelsList = null; activity = null;
        try
        {
            switch (section)
            {
                case "Overview": Overview(body); break;
                case "Chat": pane.Content = ChatPane(); UpdateLive(); return; // its own layout: the message box stays at the bottom
                case "Models": ModelsPane(body); break;
                case "System": SystemPane(body); break;
                case "Connect": ConnectPane(body); break;
                case "Keys": KeysPane(body); break;
                case "Logs": LogsPane(body); break;
                case "Settings": SettingsPane(body); break;
            }
        }
        catch (Exception ex)
        {
            // A bug in one page must never leave a dead screen: say so and offer the way out.
            Log(ex);
            body.Children.Clear();
            Header(body, section, "This page hit a problem and couldn't be shown.");
            body.Children.Add(Txt(ex.Message, 12, Bad));
            body.Children.Add(Row(10, Btn("Try again", Kind.Primary, ShowSection), Btn("Copy details for support", Kind.Normal, () => _ = CopyDiagnostics())).Also(r => r.Margin = new Thickness(0, 12, 0, 0)));
        }
        var sv = new ScrollViewer { Content = body, VerticalScrollBarVisibility = ScrollBarVisibility.Auto };
        if (section == "Logs") logScroll = sv;
        pane.Content = sv;
        UpdateLive();
    }

    void Go(string s)
    {
        section = s; note = "";
        SyncNav();
        if (shownSection != s) ShowSection();
    }

    static void Header(StackPanel b, string title, string sub) => b.Children.Add(Col(4, Txt(title, 20, Ink, true), Txt(sub, 12, Dim)).Also(s => s.Margin = new Thickness(0, 0, 0, 22)));

    bool polling;

    async Task Poll()
    {
        if (polling) return;
        polling = true;
        try
        {
            while (!quit.IsCancellationRequested && screen == Screen.Main && !engineGone)
            {
                try
                {
                    var l = await engine.Get<Live>("live");
                    var was = ServerState;
                    live = l;
                    if (was != ServerState)
                    {
                        // Server came up or went down: refresh once; otherwise update in place.
                        status = await engine.Get<Status>("status");
                        if (section is "Overview" or "Keys" && !dialogOpen) ShowSection();
                    }
                    UpdateSidebar();
                    UpdateLive();
                    if (section == "Logs") await RefreshLogs();
                }
                catch (Exception ex) when (ex is not OperationCanceledException) { }
                await Task.Delay(live?.Starting == true ? 400 : live?.Requests > 0 || ovFill != null ? 1000 : 2000);
            }
        }
        finally { polling = false; }
    }

    /// Updates whatever live readouts the current pane has.
    void UpdateLive()
    {
        UpdateOverviewLive();
        UpdateMemBars();
    }
}

static class Ext
{
    public static T Also<T>(this T t, Action<T> a) { a(t); return t; }
}

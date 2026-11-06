using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;
using Windows.System;
using static EZLM.Ui;

namespace EZLM;

// The install / switch flow: steps are built once and updated in place, so a
// download reporting progress several times a second costs a few text and
// bar updates rather than a rebuilt screen.
public sealed partial class Shell
{
    sealed class Step
    {
        public readonly string Id, Label; public readonly double Weight;
        public string State = "waiting", Sub = "", Stat = ""; public double Frac; public bool Bar, Indet;
        public Step(string id, string label, double weight) { Id = id; Label = label; Weight = weight; }
    }

    sealed class StepView
    {
        public readonly StackPanel Root;
        readonly Border mark = new() { Width = 16 };
        readonly TextBlock label, sub = Txt("", 12, Dim), stat = Txt("", 11, Dim);
        readonly BlockBar bar = new(5) { Width = 420, HorizontalAlignment = HorizontalAlignment.Left };
        string state = "";

        public StepView(Step s)
        {
            label = Txt(s.Label, 13);
            Root = Row(10, mark, Col(3, label, sub, bar, stat).Also(c => c.MaxWidth = 470));
            Update(s);
        }

        public void Update(Step s)
        {
            if (s.State != state)
            {
                state = s.State;
                mark.Child = s.State switch { "running" => new Spinner(), "ok" => Txt("✓", 13, Ok), "fail" => Txt("✗", 13, Bad), _ => Txt("·", 13, Dim) };
                label.Foreground = B(s.State == "fail" ? Bad : s.State == "waiting" ? Dim : Ink);
            }
            sub.Text = s.Sub; sub.Visibility = s.Sub == "" ? Visibility.Collapsed : Visibility.Visible;
            stat.Text = s.Stat; stat.Visibility = s.Stat == "" ? Visibility.Collapsed : Visibility.Visible;
            bar.Visibility = s.Bar ? Visibility.Visible : Visibility.Collapsed;
            if (s.Bar) { if (s.Indet) bar.Working(Signal); else bar.SetFill(s.Frac, Signal); }
        }
    }

    List<Step> steps = new();
    readonly List<StepView> stepViews = new();
    StackPanel? stepsPanel, errorPanel;
    BlockBar? overallBar;
    TextBlock? overallPct, overallStatus, overallEta;
    Button? cancelBtn;
    StackPanel? cancelRow;
    bool cancelling, installInDialog, installRunning, allowDialogClose;
    ContentDialog? installDialog;
    string? deployError;
    string installTitle = "";

    // The memory-filling panel: appears when the model starts loading.
    MemoryFillView? fill;
    StackPanel? fillHost;
    string fillName = "";
    ulong fillGpuNeed, fillRamNeed;
    Live? fillLast;

    void EnsureFill()
    {
        if (fill != null || fillHost == null) return;
        fill = new MemoryFillView(fillName == "" ? "the model" : fillName);
        fillHost.Children.Add(fill.Root);
        fillHost.Visibility = Visibility.Visible;
        _ = PollFill(fill);
    }

    async Task PollFill(MemoryFillView view)
    {
        while (installRunning && fill == view && !quit.IsCancellationRequested)
        {
            try { fillLast = await engine.Get<Live>("live"); view.Feed(fillLast, fillGpuNeed, fillRamNeed); } catch { }
            await Task.Delay(350);
        }
    }

    void NewSteps(bool quickSwitch)
    {
        steps = quickSwitch
            ? new() { new("engine", "Get the AI engine", .05), new("model", "Check the model files", .05), new("start", "Restart your server", .2), new("load", "Load the model into memory", .5), new("test", "Ask it a question", .2) }
            : new() { new("engine", "Get the AI engine", .08), new("model", "Download the model", .62), new("start", "Start your server", .04), new("load", "Load the model into memory", .16), new("test", "Ask it a question", .10) };
        deployError = null; cancelling = false; note = "";
    }

    async Task Deploy()
    {
        if (installRunning) return;
        var p = Chosen;
        if (p == null) return;
        if (screen == Screen.Main && p.Model.Id == status?.Config?.ModelId && status?.Deployed == true) return;
        if (!p.Downloaded && !p.DiskOk && scan != null)
        {
            // Never delete anything without saying so first.
            var need = (p.Download > 0 ? p.Download : p.Quant.Bytes) + (2UL << 30);
            var others = installed.Where(m => !m.Running).Select(m => m.Name).Distinct().ToList();
            var text = $"{p.Model.Name} needs about {Fmt.Gb(need)} and {Fmt.Gb(scan.Hw.DiskFree)} is free.\n\n"
                + (others.Count > 0 ? $"EZLM will first remove models you're not using ({string.Join(", ", others)}), oldest first. " : "")
                + (status?.Deployed == true ? $"If that's still not enough, it removes {status.Config?.ModelName} too, and you'd be offline until the download finishes. " : "")
                + "You can also cancel and free up space yourself.";
            if (await ShowDialog(Dialog("Make room for this model?", text, "Make room and continue")) != ContentDialogResult.Primary) return;
        }
        NewSteps(p.Downloaded);
        installTitle = screen == Screen.Main ? $"Switching to {p.Model.Name}" : $"Setting up {p.Model.Name}";
        fillName = p.Model.Name; fillGpuNeed = p.Mode == "cpu" ? 0 : p.GpuNeed; fillRamNeed = p.RamNeed;
        installRunning = true; // before the request: its first events can arrive before the reply
        try { await engine.Post("deploy", new { model_id = p.Model.Id }); }
        catch (Exception ex) { installRunning = false; note = ex.Message; if (screen == Screen.Choose) UpdateChoose(); else ShowSection(); return; }
        logo?.SetBusy(true);
        if (screen == Screen.Main) { await ShowInstallDialog(); return; }
        screen = Screen.Installing;
        host.Content = Wizard(InstallView().Also(v => { v.VerticalAlignment = VerticalAlignment.Center; }));
    }

    FrameworkElement InstallView()
    {
        overallPct = Txt("0%", 13, Signal, false, false); overallStatus = Txt("Starting", 11, Dim, false, false); overallEta = Txt("", 11, Dim, false, false);
        overallBar = new BlockBar(20); overallBar.SetFill(0, Signal);
        var head = new Grid(); head.Children.Add(Txt(installTitle, 13, Ink, true)); overallPct.HorizontalAlignment = HorizontalAlignment.Right; head.Children.Add(overallPct);
        var foot = new Grid(); foot.Children.Add(overallStatus); overallEta.HorizontalAlignment = HorizontalAlignment.Right; foot.Children.Add(overallEta);
        stepsPanel = Col(8);
        stepViews.Clear();
        foreach (var s in steps) { var v = new StepView(s); stepViews.Add(v); stepsPanel.Children.Add(v.Root); }
        errorPanel = Col(10).Also(e => e.Margin = new Thickness(0, 14, 0, 0));
        fill = null; fillLast = null;
        fillHost = Col(0).Also(f => { f.Margin = new Thickness(0, 16, 0, 0); f.Visibility = Visibility.Collapsed; });
        cancelBtn = Btn("Cancel", Kind.Normal, () => _ = CancelInstall());
        cancelRow = Row(10, cancelBtn, Txt("esc", 11, Faint).Also(t => t.VerticalAlignment = VerticalAlignment.Center)).Also(s => s.Margin = new Thickness(0, 18, 0, 0));
        RenderSteps();
        return Col(0, Col(8, head, overallBar, foot).Also(s => s.Margin = new Thickness(0, 0, 0, 26)), stepsPanel, fillHost, errorPanel, cancelRow);
    }

    void RenderSteps()
    {
        if (fill != null) foreach (var st in steps) if (st.Id is "start" or "load") st.Bar = false;
        for (int i = 0; i < stepViews.Count && i < steps.Count; i++) stepViews[i].Update(steps[i]);
        var overall = steps.Sum(s => s.Weight * s.Frac);
        overallBar?.SetFill(overall, deployError != null ? Bad : overall >= 1 ? Ok : Signal);
        if (overallPct != null) overallPct.Text = $"{(int)(overall * 100)}%";
        if (installRunning) Taskbar.Progress(Hwnd, overall);
        if (cancelBtn != null) cancelBtn.IsEnabled = installRunning && !cancelling;
    }

    void ShowDeployError(string msg)
    {
        if (errorPanel == null) return;
        errorPanel.Children.Clear();
        errorPanel.Children.Add(Txt(msg, 13, Bad));
        var hint = !installInDialog ? "Nothing was changed on your computer except what was downloaded, which is kept for next time."
            : status?.Healthy == true ? $"{status.Config?.ModelName} is back and running, so your apps keep working."
            : status?.Starting == true ? $"{status.Config?.ModelName} is back and loading; it'll answer in a minute."
            : "Your server is stopped. Press Start server on Overview to bring it back.";
        errorPanel.Children.Add(Txt(hint, 12, Dim));
        errorPanel.Children.Add(Row(10,
            Btn(installInDialog ? "Close" : "Back to the start", Kind.Primary, () => { if (installInDialog) CloseInstallDialog(); else _ = RunScan(); }),
            Btn("Report this problem", Kind.Normal, () => _ = ReportProblem(msg))));
    }

    async Task CancelInstall()
    {
        if (!installRunning || cancelling) return;
        cancelling = true;
        if (overallStatus != null) overallStatus.Text = "Cancelling…";
        RenderSteps();
        try { await engine.Post("cancel"); } catch (Exception ex) { note = ex.Message; }
    }

    async Task ShowInstallDialog()
    {
        if (dialogOpen) return;
        installInDialog = true; allowDialogClose = false;
        var view = InstallView();
        installDialog = new ContentDialog { Content = new Border { Width = 560, Child = view }, XamlRoot = root.XamlRoot, RequestedTheme = ElementTheme.Dark };
        installDialog.Resources["ContentDialogMaxWidth"] = 660.0; // the default (548) clips 560-wide content
        // Esc cancels the switch instead of hiding a dialog that's still working.
        installDialog.Closing += (sender, a) => { if (!allowDialogClose) { a.Cancel = true; if (escCloseQuestion != null) escCloseQuestion(); else if (installRunning) _ = CancelInstall(); } };
        dialogOpen = true;
        try { await installDialog.ShowAsync(); } catch (Exception ex) { Log(ex); }
        finally { dialogOpen = false; installInDialog = false; installDialog = null; }
    }

    void CloseInstallDialog()
    {
        allowDialogClose = true;
        installDialog?.Hide();
    }

    void OnDeployEvent(EngineEvent e)
    {
        if (!installRunning) return; // not a job this window started (e.g. replayed after a reconnect)
        if (e.Step == "done")
        {
            installRunning = false;
            logo?.SetBusy(false);
            if (e.State == "fail") Taskbar.Failed(Hwnd); else Taskbar.Clear(Hwnd);
            if (e.State != "cancelled") Taskbar.Flash(Hwnd);
            if (e.State == "cancelled")
            {
                note = "";
                Toast("Cancelled. What was already downloaded is kept: it picks up where it stopped next time." + (screen == Screen.Main ? " Your model kept running." : ""), Gold);
                if (installInDialog) { CloseInstallDialog(); _ = RefreshAfterJob(); }
                else { screen = Screen.Choose; chooseBuiltFor = null; ShowChoose(); }
                return;
            }
            if (cancelRow != null) cancelRow.Visibility = Visibility.Collapsed;
            if (e.State == "ok")
            {
                foreach (var s in steps) { s.Frac = 1; if (s.State != "ok") s.State = "ok"; }
                if (overallStatus != null) overallStatus.Text = "Done";
                RenderSteps();
                _ = AfterDeploy();
            }
            else
            {
                deployError = e.Msg ?? "Setup failed.";
                if (overallStatus != null) overallStatus.Text = "Stopped";
                RenderSteps();
                _ = RefreshAfterJob().ContinueWith(_ => Ui(() => ShowDeployError(deployError)));
            }
            return;
        }
        var st = steps.FirstOrDefault(s => s.Id == e.Step);
        if (st == null) return;
        if ((e.Step == "start" && e.State == "run") || e.Step == "load") EnsureFill();
        if (e.Step == "load" && e.State == "progress" && e.Total is > 0 && e.Done is { } wd) fill?.Warm(wd, e.Total.Value);
        if (e.Step == "load" && e.State == "ok") fill?.Complete(fillLast);
        switch (e.State)
        {
            case "progress":
                st.State = "running";
                if (e.Step == "test" && e.Total is null or 0)
                {
                    int.TryParse((e.Msg ?? "").TrimEnd('s'), out var asked);
                    st.Bar = true; st.Indet = true;
                    st.Stat = asked < 15 ? $"{asked}s" : $"{asked}s (some models think before they answer)";
                }
                else if (e.Step == "load" && e.Total is null or 0)
                {
                    // Server loading (seconds): the second half of the step, never backwards.
                    int.TryParse((e.Msg ?? "").TrimEnd('s'), out var secs);
                    st.Frac = Math.Max(st.Frac, 0.5 + 0.45 * (1 - Math.Exp(-secs / 25.0))); st.Bar = true; st.Indet = true; st.Stat = $"{secs}s";
                }
                else if (e.Total is > 0 && e.Done is { } d)
                {
                    var f = (double)d / e.Total.Value;
                    if (e.Step == "load") f *= 0.5; // the pre-read is the first half of loading
                    st.Frac = Math.Max(st.Frac, f); st.Bar = true; st.Indet = false;
                    var bps = e.Bps ?? 0;
                    var left = bps > 0 && e.Total > d ? (e.Total.Value - d) / bps : 0;
                    var eta = left <= 0 ? "" : left < 60 ? $"{Math.Ceiling(left)}s left" : left < 3600 ? $"{Math.Ceiling(left / 60)} min left" : $"{left / 3600:0.0} h left";
                    st.Stat = $"{Fmt.Gb(d)} of {Fmt.Gb(e.Total.Value)}   {(bps > 0 ? $"{bps / 1e6:0} MB/s" : "")}   {eta}".TrimEnd();
                    if (e.Step == "model" && overallEta != null) overallEta.Text = eta;
                }
                if (overallStatus != null && !cancelling) overallStatus.Text = st.Label;
                break;
            case "run":
                st.State = "running"; st.Sub = e.Msg ?? "";
                if (e.Step is "start" or "test") { st.Bar = true; st.Indet = true; }
                if (overallStatus != null && !cancelling) overallStatus.Text = st.Label;
                break;
            case "info": st.Sub = e.Msg ?? ""; break;
            case "ok":
                st.State = "ok"; st.Frac = 1; st.Bar = false; st.Stat = "";
                st.Sub = e.Step == "test" ? "Answered" : e.Msg ?? "";
                if (e.Step == "test") reply = e.Msg ?? "";
                if (e.Step == "model" && overallEta != null) overallEta.Text = "";
                break;
            case "fail": st.State = "fail"; st.Bar = false; st.Sub = e.Msg ?? ""; break;
        }
        RenderSteps();
    }

    async Task RefreshAfterJob()
    {
        try { status = await engine.Get<Status>("status"); } catch { }
        scan = null; chooseBuiltFor = null;
        await RefreshInstalled();
        if (screen == Screen.Main) { UpdateSidebar(); ShowSection(); }
    }

    async Task AfterDeploy()
    {
        await Task.Delay(500);
        try { status = await engine.Get<Status>("status"); } catch { }
        logo?.Flash();
        scan = null; chooseBuiltFor = null; // memory use changed: plans are stale
        if (installInDialog)
        {
            errorPanel?.Children.Clear();
            var msg = installTitle.StartsWith("Applying") ? "Done. Your URL, keys and connected apps didn't change." : "Switched. Your URL, keys and connected apps didn't change.";
            errorPanel?.Children.Add(Runs(13, ("✓ ", Ok), (msg, Ink)));
            Toast($"Now running {status?.Config?.ModelName}.", Ok);
            var done = Btn("Done  ↵", Kind.Primary, CloseInstallDialog);
            errorPanel?.Children.Add(done);
            done.Focus(FocusState.Programmatic);
            await RefreshInstalled();
            UpdateSidebar();
            ShowSection();
            return;
        }
        screen = Screen.Ready;
        ShowReady();
    }

    // MARK: ready

    void ShowReady()
    {
        var c = status?.Config;
        var content = Col(0);
        content.VerticalAlignment = VerticalAlignment.Center;
        content.Children.Add(Row(12, new Border { Width = 10, Height = 10, CornerRadius = new CornerRadius(5), Background = B(Ok), VerticalAlignment = VerticalAlignment.Center }, Txt("Your AI is live", 24, Ink, true)));
        content.Children.Add(Txt("Use this URL and key in any app that supports the OpenAI API.", 12, Dim).Also(t => t.Margin = new Thickness(0, 8, 0, 20)));
        if (c != null) content.Children.Add(Endpoint(status!.LocalUrl ?? "", c.Key, true));
        if (reply != "")
        {
            var r = Txt("", 13);
            content.Children.Add(Txt("It says:", 12, Dim).Also(t => t.Margin = new Thickness(0, 20, 0, 6)));
            content.Children.Add(new Border { BorderBrush = B(Faint), BorderThickness = new Thickness(2, 0, 0, 0), Padding = new Thickness(14, 0, 0, 0), Child = r });
            Typer.Type(r, reply, 12);
        }
        if (c != null)
        {
            content.Children.Add(Txt("Start using it", 13, Ink, true).Also(t => t.Margin = new Thickness(0, 26, 0, 4)));
            content.Children.Add(Txt("Each button opens your model in that app, setting the app up first if it isn't yet.", 12, Dim).Also(t => t.Margin = new Thickness(0, 0, 0, 10)));
            content.Children.Add(LaunchButtons(c));
        }
        content.Children.Add(Btn("Open EZLM  ↵", Kind.Primary, EnterMain).Also(b => b.Margin = new Thickness(0, 20, 0, 0)));
        host.Content = Wizard(content);
    }

    UIElement Endpoint(string url, string key, bool animate)
    {
        var col = Col(0);
        col.Children.Add(Hairline());
        foreach (var (k, shown, value) in new[] { ("URL", url, url), ("Key", Fmt.Masked(key), key) })
        {
            var g = new Grid { Padding = new Thickness(0, 10, 0, 10) };
            g.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(48) });
            g.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
            g.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
            g.Children.Add(Txt(k, 13, Dim));
            var v = Txt(animate ? "" : shown); v.IsTextSelectionEnabled = true; Grid.SetColumn(v, 1); g.Children.Add(v);
            if (animate) Typer.Type(v, shown, 16);
            g.Children.Add(CopyBtn(value, $"Copy {k}").Also(b => Grid.SetColumn(b, 2)));
            col.Children.Add(g); col.Children.Add(Hairline());
        }
        return col;
    }

    static Button CopyBtn(string value, string name)
    {
        Button? cb = null;
        cb = Btn("Copy", Kind.Small, async () =>
        {
            Copy(value);
            ((TextBlock)cb!.Content).Text = "Copied";
            await Task.Delay(1400);
            ((TextBlock)cb.Content).Text = "Copy";
        });
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(cb, name);
        return cb;
    }
}

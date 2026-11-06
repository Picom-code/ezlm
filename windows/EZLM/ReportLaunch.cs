using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Windows.System;
using static EZLM.Ui;

namespace EZLM;

public class ReportResult { public bool Sent { get; set; } public string Url { get; set; } = ""; public int Number { get; set; } }

// One-click "Open OpenCode / Open Hermes / Open DeepSeek Harness", the app
// logs behind "Show details", and the "Report a problem" form.
public sealed partial class Shell
{
    bool launching;
    bool launchFailed;
    string launchNote = "";
    Target? launchTarget; // the app the newest launch was for
    StackPanel? launchStatus;

    /// Buttons that open the user's model in each app (setting the app up
    /// first if it isn't yet) plus the built-in chat: one per launch mode, the
    /// app's logo at 16 px, wrapping when the window is narrow.
    UIElement LaunchButtons(ServerConfig c, bool withChat = true)
    {
        var row = new FlowPanel { Spacing = 10, LineSpacing = 10 };
        if (withChat) row.Children.Add(Btn("Chat with it", Kind.Normal, () => { if (screen != Screen.Main) EnterMain(); Go("Chat"); }));
        foreach (var t in (status?.Targets ?? new()).Where(t => t.Supported))
            foreach (var (m, i) in t.Modes.Select((m, i) => (m, i)))
            {
                // The first mode is the app's main button; others are smaller.
                var b = Btn(m.Label, i == 0 ? Kind.Normal : Kind.Small, () => _ = LaunchApp(t, m.Id));
                if (Marks.HasAppLogo(t.LogoKey) && b.Content is TextBlock label)
                {
                    b.Content = null;
                    label.VerticalAlignment = VerticalAlignment.Center;
                    b.Content = Row(8, Marks.AppLogo(t.LogoKey, 16), label);
                }
                b.IsEnabled = !launching && ServerState == 2;
                Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(b, m.Label);
                row.Children.Add(b);
            }
        launchStatus = Col(8);
        PaintLaunchStatus();
        return Col(8, row, launchStatus);
    }

    /// What the launch is doing (following the job's steps), or why it failed
    /// with the way to the details.
    void PaintLaunchStatus()
    {
        if (launchStatus == null) return;
        launchStatus.Children.Clear();
        if (launchNote == "") return;
        var text = Txt(launchNote, 12, launchFailed ? Bad : Dim);
        text.IsTextSelectionEnabled = launchFailed;
        launchStatus.Children.Add(launching ? Row(8, new Spinner(Dim).Also(x => x.VerticalAlignment = VerticalAlignment.Top), text) : text);
        if (launchFailed && !launching && launchTarget is { } t)
        {
            var acts = new FlowPanel { Spacing = 8, LineSpacing = 6 };
            // An app that needs 64K: the fix first, as on its tile.
            if (t.Shown == "needs_64k")
                acts.Children.Add(status?.Hermes64 is { Fits: false } && t.Id == "hermes"
                    ? Btn("Pick a smaller model", Kind.Normal, () => Go("Models"))
                    : Btn("Connect again", Kind.Normal, () => { Go("Connect"); selectedApps.Clear(); selectedApps.Add(t.Id); _ = ConnectApps(); }));
            acts.Children.Add(Btn("Show details", Kind.Small, () => _ = ShowAppLog(t)));
            acts.Children.Add(Btn("Report a problem", Kind.Small, () => _ = ReportAppProblem(t, launchNote)));
            launchStatus.Children.Add(acts);
        }
    }

    async Task LaunchApp(Target t, string mode)
    {
        if (launching) return;
        launching = true; launchFailed = false; launchTarget = t;
        launchNote = $"Opening {t.Name}…";
        RefreshLaunchViews();
        try { await engine.Post("launch", string.IsNullOrEmpty(mode) ? (object)new { id = t.Id } : new { id = t.Id, mode }); }
        catch (Exception ex) { launching = false; launchFailed = true; launchNote = ex.Message; RefreshLaunchViews(); }
    }

    void OnLaunchEvent(EngineEvent e)
    {
        if (!launching) return; // not one this window started
        if (e.Step == "done")
        {
            launching = false;
            launchFailed = e.State == "fail";
            if (e.State == "ok") { Toast(string.IsNullOrEmpty(launchNote) ? "Opened." : launchNote, Ok); launchNote = ""; }
            else if (e.State == "cancelled") launchNote = "Cancelled.";
            else
            {
                launchNote = string.IsNullOrEmpty(e.Msg) ? $"Couldn't open {launchTarget?.Name ?? "it"}." : e.Msg!;
                Toast(launchNote, Bad);
            }
            RefreshLaunchViews();
            // The app may be installed, connected or broken now.
            _ = engine.Get<Status>("status").ContinueWith(r => Ui(() =>
            {
                if (!r.IsCompletedSuccessfully) return;
                status = r.Result;
                if (launchTarget != null) launchTarget = status.Targets.FirstOrDefault(x => x.Id == launchTarget.Id) ?? launchTarget;
                RefreshLaunchViews();
            }));
            return;
        }
        if (!string.IsNullOrEmpty(e.Msg))
            launchNote = e.State is "run" or "info" ? e.Msg!.TrimEnd('…', '.') + "…" : e.Msg!;
        PaintLaunchStatus();
    }

    /// Re-renders whichever screen shows launch buttons or app states.
    void RefreshLaunchViews()
    {
        if (screen == Screen.Main && section is "Overview" or "Connect") ShowSection();
        else if (screen == Screen.Ready) ShowReady();
        else PaintLaunchStatus();
    }

    // MARK: app logs

    ContentDialog? appLogDialog;

    /// "Show details": the end of the app's connection log (keys already removed by the engine).
    async Task ShowAppLog(Target t)
    {
        if (dialogOpen) return;
        List<string> lines;
        try { lines = (await engine.Get<LogLines>($"integrate/log?id={Uri.EscapeDataString(t.Id)}")).Lines; }
        catch (Exception ex) { Toast("Couldn't read the log: " + ex.Message, Bad); return; }
        var text = HideKeys(string.Join("\n", lines));
        var body = Txt(lines.Count == 0 ? "Nothing logged yet." : text, 11, lines.Count == 0 ? Dim : Ink);
        body.IsTextSelectionEnabled = true;
        var d = new ContentDialog
        {
            Title = $"{t.Name}: connection log",
            Content = Col(10,
                Txt($"What EZLM did and what {t.Name} said, newest last. Keys are never written here.", 12, Dim),
                new ScrollViewer { Content = body, MaxHeight = 340 }),
            PrimaryButtonText = lines.Count == 0 ? "" : "Copy", SecondaryButtonText = "Report a problem", CloseButtonText = "Close",
            DefaultButton = ContentDialogButton.Close, XamlRoot = root.XamlRoot, RequestedTheme = ElementTheme.Dark,
        };
        d.Resources["ContentDialogMaxWidth"] = 760.0;
        appLogDialog = d;
        var r = await ShowDialog(d);
        appLogDialog = null;
        if (r == ContentDialogResult.Primary) { Copy(text); Toast("Copied (your keys are hidden).", Ok); }
        else if (r == ContentDialogResult.Secondary) await ReportAppProblem(t, AppText.Status(t).Text, lines);
    }

    /// "Report a problem" for one app: the reason first, then the end of its log
    /// (same text as the Mac app and the web page).
    async Task ReportAppProblem(Target t, string reason, List<string>? lines = null)
    {
        if (lines == null)
            try { lines = (await engine.Get<LogLines>($"integrate/log?id={Uri.EscapeDataString(t.Id)}")).Lines; } catch { lines = new(); }
        var ctx = $"{t.Name}: {reason}";
        var tail = lines.TakeLast(40).ToList();
        if (tail.Count > 0) ctx += $"\n\n{t.Name} log (last {tail.Count} lines):\n" + string.Join("\n", tail);
        await ReportProblem(HideKeys(ctx));
    }

    /// Never hand out API keys: anything shaped like an EZLM key is shortened.
    static string HideKeys(string s) => System.Text.RegularExpressions.Regex.Replace(s, @"ezlm-[A-Za-z0-9]{6,}", m => m.Value[..9] + "…");

    // MARK: problem reports

    async Task ReportProblem(string? context = null)
    {
        if (dialogOpen) return;
        var box = new TextBox
        {
            AcceptsReturn = true, TextWrapping = TextWrapping.Wrap, Height = 130, FontFamily = Mono, CornerRadius = new CornerRadius(0),
            PlaceholderText = "What happened? What did you expect instead?", Text = context ?? "",
        };
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(box, "What happened");
        var details = new CheckBox { IsChecked = true, Content = Txt("Include technical details: your hardware, versions and recent logs. Keys and your files are never included.", 12) };
        var note = Txt("", 12, Bad);
        var d = new ContentDialog
        {
            Title = "Report a problem",
            Content = Col(10, Txt("This posts on EZLM's public GitHub page, where the people testing EZLM read it. Thanks for helping.", 12, Dim), box, details, note),
            PrimaryButtonText = "Send report", CloseButtonText = "Cancel", DefaultButton = ContentDialogButton.Primary,
            XamlRoot = root.XamlRoot, RequestedTheme = ElementTheme.Dark,
        };
        d.Resources["ContentDialogMaxWidth"] = 640.0;
        d.PrimaryButtonClick += (_, a) => { if (string.IsNullOrWhiteSpace(box.Text)) { a.Cancel = true; note.Text = "Say what happened first, even in a few words."; } };
        if (await ShowDialog(d) != ContentDialogResult.Primary) return;
        Toast("Sending your report…");
        try
        {
            var appLog = "";
            try
            {
                var p = System.IO.Path.Combine(Engine.DataDir, "app.log");
                if (File.Exists(p)) { var all = File.ReadAllText(p); appLog = all.Length > 6000 ? all[^6000..] : all; }
            }
            catch { }
            var r = await engine.Post<ReportResult>("report", new { text = box.Text.Trim(), details = details.IsChecked == true, app_log = appLog });
            if (r.Sent) Toast($"Sent. Thank you! (report #{r.Number})", Ok);
            else
            {
                await Launcher.LaunchUriAsync(new Uri(r.Url));
                Toast("Your report is filled in on GitHub: press Submit there to send it.", Gold);
            }
        }
        catch (Exception ex) { Log(ex); Toast("Couldn't send the report: " + ex.Message, Bad); }
    }
}

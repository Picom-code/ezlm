using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media.Imaging;
using static EZLM.Ui;

namespace EZLM;

public class MemApp
{
    public string Id { get; set; } = "";
    public string Name { get; set; } = "";
    public string Exe { get; set; } = "";
    public List<int>? Pids { get; set; }
    public ulong Ram { get; set; }
    public ulong Gpu { get; set; }
    public bool Windows { get; set; }
    public string Kind { get; set; } = "app";
    public string Note { get; set; } = "";
}

public class MemAppList { public List<MemApp> Apps { get; set; } = new(); }
public class CloseResult { public bool Closed { get; set; } public int Remaining { get; set; } public ulong Freed { get; set; } public bool NeedForce { get; set; } }

// Model mode: walks through the apps using the most memory, one at a time,
// and closes only the ones the user says yes to. Gentle close first (like
// clicking the window's X); force-close only when asked, with a warning.
public sealed partial class Shell
{
    ContentDialog? mmDialog;
    bool mmBusy, mmAllowClose;
    TaskCompletionSource mmGone = new();

    async Task ModelMode(Action? after = null)
    {
        if (dialogOpen) return;
        var body = Col(14);
        var root = new Border { Width = 540, Child = body };
        mmDialog = new ContentDialog { Content = root, XamlRoot = this.root.XamlRoot, RequestedTheme = ElementTheme.Dark };
        mmDialog.Resources["ContentDialogMaxWidth"] = 640.0; // the default (548) clips 540-wide content
        mmAllowClose = false;
        mmDialog.Closing += (sender, a) => { if (mmBusy && !mmAllowClose) a.Cancel = true; };
        dialogOpen = true;
        mmGone = new TaskCompletionSource();
        var walk = Walk(body);
        try { await mmDialog.ShowAsync(); } catch (Exception ex) { Log(ex); }
        // Closed, by a button or by Esc in the middle: a question still waiting
        // gets "stop", and other dialogs can open again right away.
        mmGone.TrySetResult();
        dialogOpen = false; mmDialog = null;
        try { await walk; } catch (Exception ex) { Log(ex); }
        after?.Invoke();
    }

    /// The button picked, or "stop" once the dialog is gone.
    async Task<string> Answer(TaskCompletionSource<string> pick) =>
        await Task.WhenAny(pick.Task, mmGone.Task) == pick.Task ? pick.Task.Result : "stop";

    void CloseModelMode() { mmAllowClose = true; mmDialog?.Hide(); }

    async Task Walk(StackPanel body)
    {
        UIElement Head(string sub) => Col(4, Txt("Model mode", 18, Ink, true), Txt(sub, 12, Dim));
        body.Children.Clear();
        body.Children.Add(Head("Closing apps you're not using gives your AI more memory. EZLM asks about each one; nothing closes without your OK."));
        body.Children.Add(Row(10, new Spinner(), Txt("Looking at what's using memory…", 13, Dim)));

        List<MemApp> apps;
        try { apps = (await engine.Get<MemAppList>("apps")).Apps; }
        catch (Exception ex) { body.Children.Add(Txt(ex.Message, 12, Bad)); body.Children.Add(Btn("Close", Kind.Primary, CloseModelMode)); return; }

        if (apps.Count == 0)
        {
            body.Children.Clear();
            body.Children.Add(Head("Nothing big is running. Your AI already has all the memory this computer can give it."));
            body.Children.Add(Btn("Done  ↵", Kind.Primary, CloseModelMode).Also(b => b.Focus(FocusState.Programmatic)));
            return;
        }

        ulong potential = (ulong)apps.Sum(a => (double)(a.Ram + a.Gpu)), freed = 0, freedGpu = 0;
        int closedCount = 0;
        // Fresh elements per step: WinUI won't re-parent an element whose old
        // parent left the tree.
        BlockBar? freedBar = null;
        TextBlock? freedText = null;
        double Frac() => potential > 0 ? (double)(freed + freedGpu) / potential : 1;
        UIElement Tally()
        {
            freedBar = new BlockBar(8); freedBar.SetFill(Frac(), Ok);
            freedText = Txt(Fmt.Gb(freed + freedGpu), 12, Ok, false, false);
            var g = new Grid { ColumnSpacing = 12, Margin = new Thickness(0, 6, 0, 0) };
            g.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
            g.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
            g.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
            g.Children.Add(Txt("Freed so far", 12, Dim, false, false));
            freedBar.VerticalAlignment = VerticalAlignment.Center; Grid.SetColumn(freedBar, 1); g.Children.Add(freedBar);
            Grid.SetColumn(freedText, 2); g.Children.Add(freedText);
            return Col(10, Hairline(), g);
        }

        for (int i = 0; i < apps.Count; i++)
        {
            var app = apps[i];
            var decision = new TaskCompletionSource<string>();
            body.Children.Clear();
            var head = new Grid();
            head.Children.Add(Txt("Model mode", 18, Ink, true));
            head.Children.Add(Txt($"{i + 1} of {apps.Count}", 12, Dim, false, false).Also(t => { t.HorizontalAlignment = HorizontalAlignment.Right; t.VerticalAlignment = VerticalAlignment.Center; }));
            body.Children.Add(head);

            var icon = new Border { Width = 40, Height = 40, Child = Txt(app.Name.Length > 0 ? app.Name[..1] : "?", 18, Dim, true).Also(t => { t.HorizontalAlignment = HorizontalAlignment.Center; t.VerticalAlignment = VerticalAlignment.Center; }), BorderBrush = B(Faint), BorderThickness = new Thickness(1) };
            _ = LoadIcon(icon, app.Exe);
            var usage = $"{Fmt.Gb(app.Ram)} of memory" + (app.Gpu >= (64UL << 20) ? $", {Fmt.Gb(app.Gpu)} of graphics memory" : "");
            body.Children.Add(Row(14, icon, Col(3, Txt(app.Name, 16, Ink, true), Txt(usage, 12, Dim))).Also(r => r.Margin = new Thickness(0, 4, 0, 0)));
            body.Children.Add(Txt(app.Note, 12, app.Kind == "ai" ? Gold : Dim));

            var actions = Row(10);
            var progress = Col(6);
            var verb = app.Kind == "background" ? "Stop" : "Close";
            var go = Btn($"{verb} {app.Name}  ↵", Kind.Primary, () => decision.TrySetResult("close"));
            var keep = Btn("Keep it", Kind.Normal, () => decision.TrySetResult("keep"));
            var stop = Btn("Stop here", Kind.Small, () => decision.TrySetResult("stop"));
            actions.Children.Add(go); actions.Children.Add(keep); actions.Children.Add(stop.Also(s => s.VerticalAlignment = VerticalAlignment.Center));
            body.Children.Add(actions);
            body.Children.Add(progress);
            body.Children.Add(Tally());
            go.Focus(FocusState.Programmatic);

            var choice = await Answer(decision);
            if (choice == "stop") break;
            if (choice == "keep") continue;

            // Close it: gently first, then offer force if it's still running.
            bool force = !app.Windows && app.Kind == "background";
            while (true)
            {
                actions.Visibility = Visibility.Collapsed;
                progress.Children.Clear();
                progress.Children.Add(Row(10, new Spinner(), Txt($"{(force ? "Stopping" : "Closing")} {app.Name}…", 13, Dim)));
                mmBusy = true;
                CloseResult r;
                try { r = await engine.Post<CloseResult>("apps/close", new { id = app.Id, force }); }
                catch (Exception ex) { r = new CloseResult { NeedForce = false }; progress.Children.Clear(); progress.Children.Add(Txt(ex.Message, 12, Bad)); }
                finally { mmBusy = false; }
                if (r.Closed || r.Freed > 0)
                {
                    freed += r.Freed > 0 ? r.Freed : app.Ram;
                    freedGpu += app.Gpu;
                    closedCount++;
                    freedBar?.SetFill(Frac(), Ok);
                    if (freedText != null) freedText.Text = Fmt.Gb(freed + freedGpu);
                    progress.Children.Clear();
                    progress.Children.Add(Runs(13, ("✓ ", Ok), ($"{app.Name} closed. Freed {Fmt.Gb((r.Freed > 0 ? r.Freed : app.Ram) + app.Gpu)}.", Ink)));
                    await Task.Delay(900);
                    break;
                }
                if (!r.NeedForce) break; // error shown above
                // Still running: explain, and only force-close if they say so.
                progress.Children.Clear();
                progress.Children.Add(Txt(app.Windows ? $"{app.Name} is still running (it may have asked to save something, or it keeps going in the background)." : $"{app.Name} has no window to close.", 13, Gold));
                var again = new TaskCompletionSource<string>();
                var forceBtn = Btn($"Force close {app.Name}", Kind.Danger, () => again.TrySetResult("force"));
                var keep2 = Btn("Keep it", Kind.Normal, () => again.TrySetResult("keep"));
                progress.Children.Add(Txt("Force-closing stops it right away: anything unsaved in it is lost.", 12, Dim));
                progress.Children.Add(Row(10, forceBtn, keep2));
                if (await Answer(again) != "force") break;
                force = true;
            }
        }

        // Finished: say what it bought, and offer to use it.
        body.Children.Clear();
        body.Children.Add(Txt("Model mode", 18, Ink, true));
        if (closedCount == 0)
        {
            body.Children.Add(Txt("Nothing was closed. You can run Model mode again any time from Overview or System.", 13, Dim));
            body.Children.Add(Btn("Done  ↵", Kind.Primary, CloseModelMode).Also(b => b.Focus(FocusState.Programmatic)));
            return;
        }
        body.Children.Add(Runs(22, ($"Freed {Fmt.Gb(freed + freedGpu)}", Ok)).Also(t => t.FontWeight = Microsoft.UI.Text.FontWeights.Bold));
        body.Children.Add(Txt($"Closed {closedCount} app{(closedCount == 1 ? "" : "s")}." + (freedGpu >= (256UL << 20) ? $" {Fmt.Gb(freedGpu)} of that was graphics memory, the most valuable kind for AI." : ""), 13, Dim));
        var next = Col(10);
        body.Children.Add(next);
        next.Children.Add(Row(10, new Spinner(), Txt("Checking what your AI can do with the room…", 12, Dim)));
        try { status = await engine.Get<Status>("status"); } catch { }
        next.Children.Clear();
        if (screen == Screen.Main && status?.Tune is { } tune && status.Config is { } cfg)
        {
            next.Children.Add(Txt("Your model can use it: " + tune.Why, 13, Ink));
            next.Children.Add(Row(10,
                Btn("Apply the faster setup  ↵", Kind.Primary, () => { CloseModelMode(); _ = Task.Delay(300).ContinueWith(_ => Ui(() => _ = ApplyTune(cfg, tune))); }).Also(b => b.Focus(FocusState.Programmatic)),
                Btn("Not now", Kind.Normal, CloseModelMode)));
        }
        else if (screen != Screen.Main)
        {
            next.Children.Add(Txt("EZLM will compare models again with the extra room.", 13, Dim));
            next.Children.Add(Btn("See what fits now  ↵", Kind.Primary, CloseModelMode).Also(b => b.Focus(FocusState.Programmatic)));
        }
        else
        {
            next.Children.Add(Txt("Your model has more breathing room now. Models that didn't fit before may fit now: have a look in Models.", 13, Dim));
            next.Children.Add(Row(10, Btn("Done  ↵", Kind.Primary, CloseModelMode).Also(b => b.Focus(FocusState.Programmatic)), Btn("Open Models", Kind.Normal, () => { CloseModelMode(); scan = null; Go("Models"); })));
        }
        scan = null; chooseBuiltFor = null; // memory changed
    }

    /// The app's real icon, from Windows' own thumbnail for its program file.
    static async Task LoadIcon(Border host, string exe)
    {
        try
        {
            if (string.IsNullOrEmpty(exe) || !File.Exists(exe)) return;
            var file = await Windows.Storage.StorageFile.GetFileFromPathAsync(exe);
            using var thumb = await file.GetThumbnailAsync(Windows.Storage.FileProperties.ThumbnailMode.SingleItem, 64);
            if (thumb == null) return;
            var bmp = new BitmapImage();
            await bmp.SetSourceAsync(thumb);
            host.BorderThickness = new Thickness(0);
            host.Child = new Image { Source = bmp, Width = 40, Height = 40 };
        }
        catch { }
    }
}

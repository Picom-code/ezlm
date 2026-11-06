using System.Runtime.InteropServices.WindowsRuntime;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media.Imaging;
using Windows.Graphics.Imaging;
using Windows.Storage.Streams;

namespace EZLM;

/// A developer check that shows nothing on screen: with --demo --snap <folder>
/// the window opens off-screen, unfocused and out of the taskbar, walks
/// through Chat with the demo model and then the connected apps (Connect,
/// the launch buttons, a failed launch, at 100% and 150% scale), saves each
/// step as a PNG, and quits. --snap-only apps|chat runs one half;
/// --demo-apps <states> sets the demo apps' starting states (EZLM_DEMO_APPS).
/// It only works in demo mode, so it never touches anyone's real setup.
public sealed partial class Shell
{
    static string? SnapArg(string name)
    {
        var a = Environment.GetCommandLineArgs();
        int i = Array.IndexOf(a, name);
        return i >= 0 && i + 1 < a.Length ? a[i + 1] : null;
    }

    public static string? SnapDir
    {
        get
        {
            var a = Environment.GetCommandLineArgs();
            int i = Array.IndexOf(a, "--snap");
            return Engine.Demo && i >= 0 && i + 1 < a.Length ? a[i + 1] : null;
        }
    }

    public void StartHidden()
    {
        AppWindow.IsShownInSwitchers = false;
        AppWindow.Move(new Windows.Graphics.PointInt32(-32000, -32000));
        AppWindow.Show(false); // shown, never activated
        _ = SnapRun(SnapDir!);
    }

    async Task SnapRun(string dir)
    {
        try
        {
            Directory.CreateDirectory(dir);
            for (int i = 0; i < 150 && (screen == Screen.Booting || screen == Screen.Scanning); i++) await Task.Delay(200);
            if (screen == Screen.Choose) { await Task.Delay(1200); await Shot(dir, "0-choose"); }
            if (status?.Deployed != true)
            {
                await engine.Post("deploy", new { model_id = "" }); // the demo's recommended model
                for (int i = 0; i < 300 && status?.Deployed != true; i++) { await Task.Delay(300); try { status = await engine.Get<Status>("status"); } catch { } }
            }
            EnterMain();
            var only = SnapArg("--snap-only");
            if (only != "apps") await SnapChat(dir);
            if (only != "chat") await SnapApps(dir);
        }
        catch (Exception ex) { Log(ex); File.WriteAllText(Path.Combine(dir, "error.txt"), ex.ToString()); }
        finally { Application.Current.Exit(); }
    }

    async Task SnapChat(string dir)
    {
        Go("Chat");
        await Task.Delay(1600); await Shot(dir, "1-chat-welcome");
        _ = SendChat("Show me what an answer looks like. My name is Sam and I live in Austin.");
        await Task.Delay(1500); await Shot(dir, "2-chat-thinking");
        await Task.Delay(2600); await Shot(dir, "3-chat-writing");
        for (int i = 0; i < 300 && chatCts != null; i++) await Task.Delay(200);
        await Task.Delay(1800); await Shot(dir, "4-chat-done");
        // With a test server, the quiet look for facts runs 8 s after the answer.
        if (TestServer != null)
        {
            for (int i = 0; i < 120 && (memCts != null || memWorking || chat.LastOrDefault()?.Remembered.Count == 0) && i < 60; i++) await Task.Delay(250);
            await Shot(dir, "4b-chat-after-memory");
            File.WriteAllText(Path.Combine(dir, "chat.txt"), string.Join("\n---\n", chat.Select(m => (m.User ? "USER: " : "MODEL: ") + m.Text + (m.Error != "" ? " [error: " + m.Error + "]" : "") + $" [rate {m.Rate:0} tokens {m.Tokens}] remembered: {string.Join(" | ", m.Remembered)}")));
        }
        chatDraft = "/"; // the command palette
        ShowSection();
        await Task.Delay(800); await Shot(dir, "5-chat-typing");
        // Read-aloud works without package identity: synthesize (never play) one sentence.
        try
        {
            using var synth = new Windows.Media.SpeechSynthesis.SpeechSynthesizer();
            using var voice = await synth.SynthesizeTextToStreamAsync("This is how answers sound.");
            File.WriteAllText(Path.Combine(dir, "voice.txt"), $"{voice.Size} bytes, {voice.ContentType}, voice {synth.Voice?.DisplayName}");
        }
        catch (Exception ex) { File.WriteAllText(Path.Combine(dir, "voice.txt"), "failed: " + ex.Message); }
        // The Memory panel (a dialog: rendered on its own).
        Memory.Add("The user's name is Sam."); Memory.Add("The user lives in Austin and works as a teacher.");
        Memory.Instructions = "Keep answers short and friendly.";
        _ = ShowMemory();
        await Task.Delay(1500);
        // A dialog lives in its own layer above the window: render that layer whole.
        foreach (var pop in Microsoft.UI.Xaml.Media.VisualTreeHelper.GetOpenPopupsForXamlRoot(root.XamlRoot))
            if (pop.Child is UIElement layer) { await Shot(dir, "7-memory", layer); break; }
        memDialog?.Hide();
        await Task.Delay(500);
        Go("Overview");
        await Task.Delay(1200); await Shot(dir, "6-overview");
    }

    /// The connected apps: tiles in each state, the launch buttons, a launch
    /// that fails and its details, connecting one, a narrow window, Settings ›
    /// About, then the same screens at 150% scale.
    async Task SnapApps(string dir)
    {
        Target? App(string id) => status?.Targets.FirstOrDefault(t => t.Id == id);
        async Task Settle(int ms = 1200) { await Task.Delay(ms); }
        async Task Until(Func<bool> done, int seconds) { for (int i = 0; i < seconds * 5 && !done(); i++) await Task.Delay(200); }
        void Into(UIElement? e) => e?.StartBringIntoView(new BringIntoViewOptions { AnimationDesired = false, VerticalAlignmentRatio = 0.35 });

        try { status = await engine.Get<Status>("status"); } catch { }
        var shots = new List<string>();
        async Task Snap(string name, double scale = 1, UIElement? what = null) { await Shot(dir, name, what, scale); shots.Add(name); }

        Go("Connect"); ShowSection();
        await Settle(); await Snap("a1-connect");

        Go("Overview");
        await Settle(); Into(launchStatus); await Settle(600); await Snap("a2-overview-launch");

        if (App("dsh") is { } dsh)
        {
            _ = LaunchApp(dsh, dsh.Modes[0].Id);
            await Settle(1000); Into(launchStatus); await Settle(300); await Snap("a3-launch-progress");
            await Until(() => !launching, 15);
            await Settle(1500); Into(launchStatus); await Settle(600); await Snap(launchFailed ? "a4-launch-failed" : "a4-launch-opened");
            _ = ShowAppLog(launchTarget ?? dsh);
            await Settle(1500);
            foreach (var pop in Microsoft.UI.Xaml.Media.VisualTreeHelper.GetOpenPopupsForXamlRoot(root.XamlRoot))
                if (pop.Child is UIElement layer) { await Snap("a5-app-log", 1, layer); break; }
            appLogDialog?.Hide();
            await Settle(600);
        }

        Go("Connect"); ShowSection();
        await Settle(); await Snap("a6-connect-broken");

        if (App("opencode") is { } oc)
        {
            selectedApps.Clear(); selectedApps.Add(oc.Id);
            _ = ConnectApps();
            await Settle(2200); await Snap("a7-connect-working");
            await Until(() => !connecting, 20);
            await Settle(1500); await Snap("a8-connect-done");
        }

        Go("Overview");
        await Settle(); Into(launchStatus); await Settle(600); await Snap("a9-overview-connected");

        // A narrow window: the tiles wrap to two rows.
        var size = AppWindow.Size;
        AppWindow.Resize(new Windows.Graphics.SizeInt32(940, size.Height));
        Go("Connect"); ShowSection();
        await Settle(1500); await Snap("b1-connect-narrow");
        Go("Overview");
        await Settle(); Into(launchStatus); await Settle(600); await Snap("b2-overview-narrow");
        AppWindow.Resize(size);

        Go("Settings");
        await Settle(2000);
        if (pane?.Content is ScrollViewer sv) { sv.UpdateLayout(); sv.ChangeView(null, sv.ScrollableHeight, null, true); }
        await Settle(600); await Snap("b3-settings-about");

        // 150%: the same layout drawn at 1.5x, text and logos rasterised for it.
        var frame = Scaled(1.5);
        Go("Connect"); ShowSection();
        await Settle(1500); await Snap("c1-connect@150", 1, frame);
        Go("Overview"); ShowSection();
        await Settle(); Into(launchStatus); await Settle(600); await Snap("c2-overview@150", 1, frame);
        frame = Scaled(1.5, 940 / Marks.Scale - 16);
        Go("Connect"); ShowSection();
        await Settle(1500); await Snap("c3-connect-narrow@150", 1, frame);
        Scaled(1);

        // What each tile said, for checking without looking.
        File.WriteAllLines(Path.Combine(dir, "apps.txt"), (status?.Targets ?? new()).Select(t => $"{t.Id}: {t.Shown} | {AppText.Status(t).Text} | {AppText.VersionLine(t)} | {AppText.CheckLine(t, DateTime.UtcNow)}")
            .Append("launch: " + (launchFailed ? "failed: " : "") + launchNote).Append("shots: " + string.Join(" ", shots)));
    }

    /// Draws the window's content at scale x its size (as on a display at
    /// that scale) into an off-window frame: the layout keeps its size in
    /// view pixels while text and logos are rasterised for the scale. Returns
    /// the frame to render; Scaled(1) puts the window back.
    FrameworkElement Scaled(double scale, double? width = null)
    {
        var view = root.XamlRoot.Size; // the window's content, in view pixels
        if (Content != root)
        {
            ((Panel)root.Parent).Children.Clear();
            Content = root;
            root.Width = root.Height = double.NaN; root.RenderTransform = null; root.RasterizationScale = 1;
            root.HorizontalAlignment = HorizontalAlignment.Stretch; root.VerticalAlignment = VerticalAlignment.Stretch;
            Marks.ScaleOverride = null;
        }
        if (scale == 1) return root;
        double w = width ?? view.Width, h = view.Height;
        Marks.ScaleOverride = scale * Marks.Scale;
        Content = null;
        root.Width = w; root.Height = h;
        root.HorizontalAlignment = HorizontalAlignment.Left; root.VerticalAlignment = VerticalAlignment.Top;
        root.RenderTransform = new Microsoft.UI.Xaml.Media.ScaleTransform { ScaleX = scale, ScaleY = scale };
        root.RasterizationScale = scale * Marks.Scale;
        var frame = new Grid { Width = w * scale, Height = h * scale, Background = EZLM.Ui.B(EZLM.Ui.Black) };
        frame.Children.Add(root);
        var canvas = new Canvas();
        canvas.Children.Add(frame);
        Content = canvas;
        return frame;
    }

    async Task Shot(string dir, string name, UIElement? what = null, double scale = 1)
    {
        var rtb = new RenderTargetBitmap();
        var el = what ?? root;
        if (scale == 1) await rtb.RenderAsync(el);
        else await rtb.RenderAsync(el, (int)Math.Round(((FrameworkElement)el).ActualWidth * scale), (int)Math.Round(((FrameworkElement)el).ActualHeight * scale));
        var pixels = await rtb.GetPixelsAsync();
        using var mem = new InMemoryRandomAccessStream();
        var enc = await BitmapEncoder.CreateAsync(BitmapEncoder.PngEncoderId, mem);
        enc.SetPixelData(BitmapPixelFormat.Bgra8, BitmapAlphaMode.Premultiplied, (uint)rtb.PixelWidth, (uint)rtb.PixelHeight, 96, 96, pixels.ToArray());
        await enc.FlushAsync();
        mem.Seek(0);
        using var file = File.Create(Path.Combine(dir, name + ".png"));
        await mem.AsStreamForRead().CopyToAsync(file);
    }
}

using Microsoft.UI;
using Microsoft.UI.Text;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Documents;
using Microsoft.UI.Xaml.Media;
using Microsoft.UI.Xaml.Media.Imaging;
using Microsoft.UI.Xaml.Shapes;
using Windows.UI;

namespace EZLM;

/// Colors, fonts and small builders shared by every screen.
public static class Ui
{
    public static Color C(uint rgb, byte a = 255) => ColorHelper.FromArgb(a, (byte)(rgb >> 16), (byte)(rgb >> 8), (byte)rgb);
    public static readonly Color Black = C(0x000000), Ink = C(0xe6e6e6), Dim = C(0x6b6b6b), Faint = C(0x3a3a3a), Rule = C(0x262626),
        Signal = C(0xb9a8ff), Ok = C(0x8fe3b5), Bad = C(0xff8f86), Gold = C(0xe8c872), Panel = C(0x0e0e0e);
    public static SolidColorBrush B(Color c) => new(c);
    public static readonly FontFamily Mono = new("Cascadia Mono, Consolas");

    public static TextBlock Txt(string text, double size = 13, Color? color = null, bool bold = false, bool wrap = true) => new()
    {
        Text = text, FontFamily = Mono, FontSize = size, Foreground = B(color ?? Ink),
        FontWeight = bold ? FontWeights.Bold : FontWeights.Normal,
        TextWrapping = wrap ? TextWrapping.Wrap : TextWrapping.NoWrap, IsTextSelectionEnabled = false,
    };

    public static TextBlock Runs(double size, params (string text, Color color)[] parts)
    {
        var t = new TextBlock { FontFamily = Mono, FontSize = size, TextWrapping = TextWrapping.Wrap };
        foreach (var (text, color) in parts) t.Inlines.Add(new Run { Text = text, Foreground = B(color) });
        return t;
    }

    public enum Kind { Primary, Normal, Small, Danger }

    /// Buttons read like key hints in a terminal: square, outlined; primary is solid.
    public static Button Btn(string text, Kind kind, Action onClick)
    {
        var b = new Button
        {
            Content = new TextBlock { Text = text, FontFamily = Mono, FontSize = kind == Kind.Small ? 12 : 13, FontWeight = kind == Kind.Primary ? FontWeights.Bold : FontWeights.Normal },
            CornerRadius = new CornerRadius(0),
            BorderThickness = new Thickness(1),
            Padding = kind switch { Kind.Primary => new Thickness(20, 9, 20, 9), Kind.Small => new Thickness(9, 3, 9, 3), _ => new Thickness(14, 6, 14, 6) },
            Background = B(kind == Kind.Primary ? Ink : Colors.Transparent),
            Foreground = B(kind == Kind.Primary ? Black : kind == Kind.Small ? Dim : Ink),
            BorderBrush = B(kind == Kind.Primary ? Ink : Faint),
        };
        // Keep our colors on hover/press instead of the default accent styling.
        b.Resources["ButtonBackgroundPointerOver"] = B(kind == Kind.Primary ? C(0xffffff) : C(0x161616));
        b.Resources["ButtonBackgroundPressed"] = B(kind == Kind.Primary ? C(0xcccccc) : C(0x1e1e1e));
        b.Resources["ButtonForegroundPointerOver"] = B(kind == Kind.Primary ? Black : Ink);
        b.Resources["ButtonForegroundPressed"] = B(kind == Kind.Primary ? Black : Ink);
        b.Resources["ButtonBorderBrushPointerOver"] = B(kind == Kind.Danger ? Bad : kind == Kind.Primary ? C(0xffffff) : Dim);
        b.Resources["ButtonBackgroundDisabled"] = B(kind == Kind.Primary ? Rule : Colors.Transparent);
        b.Resources["ButtonForegroundDisabled"] = B(Faint);
        b.Resources["ButtonBorderBrushDisabled"] = B(Rule);
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(b, text.Replace("↵", "").Trim());
        b.Click += (_, _) => onClick();
        return b;
    }

    public static Border Hairline(bool vertical = false) => new()
    {
        Background = B(Rule), Height = vertical ? double.NaN : 1, Width = vertical ? 1 : double.NaN,
    };

    public static StackPanel Col(double spacing = 0, params UIElement[] kids)
    {
        var s = new StackPanel { Spacing = spacing };
        foreach (var k in kids) s.Children.Add(k);
        return s;
    }

    public static StackPanel Row(double spacing = 0, params UIElement[] kids)
    {
        var s = new StackPanel { Orientation = Orientation.Horizontal, Spacing = spacing };
        foreach (var k in kids) s.Children.Add(k);
        return s;
    }

    public static Grid Fact(string k, string v)
    {
        var g = new Grid { Margin = new Thickness(0, 1.5, 0, 1.5) };
        g.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(110) });
        g.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        var kt = Txt(k, 13, Dim); var vt = Txt(v); vt.IsTextSelectionEnabled = true;
        Grid.SetColumn(vt, 1); g.Children.Add(kt); g.Children.Add(vt);
        return g;
    }

    public static TextBlock Section(string text) { var t = Txt(text, 13, Dim); t.Margin = new Thickness(0, 22, 0, 8); return t; }

    public static void Copy(string s)
    {
        var dp = new Windows.ApplicationModel.DataTransfer.DataPackage();
        dp.SetText(s);
        Windows.ApplicationModel.DataTransfer.Clipboard.SetContent(dp);
    }
}

/// The terminal progress bar, drawn as blocks: [0,a) in First, [a,b) in Second,
/// the rest as empty blocks. It's three shapes (one per color) whatever its
/// width, and repaints only when a block actually changes color, so bars are
/// cheap to update, cheap to create in lists, and never replay an animation.
public sealed class BlockBar : Grid
{
    const double Seg = 6, Gap = 2;
    readonly Microsoft.UI.Xaml.Shapes.Path pFirst = new(), pSecond = new(), pEmpty = new();
    double a, b;
    public Color First = Ui.Dim, Second = Ui.Signal;
    DispatcherTimer? timer;
    bool indeterminate;
    DateTime started;
    (int n, int i, int j, Color f, Color s) painted = (-1, -1, -1, default, default);

    public BlockBar(double height = 12)
    {
        Height = height;
        pEmpty.Fill = Ui.B(Ui.Rule);
        pFirst.Fill = Ui.B(First);
        pSecond.Fill = Ui.B(Second);
        Children.Add(pEmpty); Children.Add(pFirst); Children.Add(pSecond);
        SizeChanged += (_, e) => { if (Math.Abs(e.NewSize.Width - e.PreviousSize.Width) > 0.5) Paint(); };
        Unloaded += (_, _) => timer?.Stop();
        Loaded += (_, _) => { if (indeterminate) timer?.Start(); };
    }

    /// Two segments: other usage then this model (fractions of the whole bar).
    public void Set(double others, double model)
    {
        indeterminate = false; timer?.Stop();
        a = Math.Clamp(double.IsFinite(others) ? others : 0, 0, 1);
        b = Math.Clamp(double.IsFinite(others + model) ? others + model : a, a, 1);
        Paint();
    }

    public void SetFill(double v, Color color) { First = Colors.Transparent; Second = color; Set(0, v); }

    /// A block of light sliding back and forth: progress of unknown length.
    public void Working(Color color)
    {
        Second = color; First = Colors.Transparent;
        if (indeterminate) return; // already sliding: don't restart the motion
        indeterminate = true; started = DateTime.Now;
        timer ??= new DispatcherTimer { Interval = TimeSpan.FromMilliseconds(50) };
        timer.Tick -= Tick; timer.Tick += Tick;
        timer.Start();
    }

    void Tick(object? s, object e)
    {
        var t = (DateTime.Now - started).TotalSeconds % 2.6 / 1.3;
        var p = t < 1 ? t : 2 - t; p = p * p * (3 - 2 * p);
        a = p * 0.78; b = a + 0.22;
        Paint();
    }

    void Paint()
    {
        double w = ActualWidth, h = Height;
        if (w <= 0 || double.IsNaN(h)) return;
        int n = (int)((w + Gap) / (Seg + Gap));
        // Block k is in a segment when its center is: first [0,i), second [i,j), empty after
        // (indeterminate: second is [i,j) and everything else empty).
        int Idx(double f) { int k = 0; while (k < n && (k * (Seg + Gap) + Seg / 2) / w < f) k++; return k; }
        int i = Idx(a), j = Math.Max(i, Idx(b));
        var key = (n, i, j, First, Second);
        if (key == painted) return;
        painted = key;
        GeometryGroup Blocks(int from, int to)
        {
            var g = new GeometryGroup();
            for (int k = from; k < to; k++) g.Children.Add(new RectangleGeometry { Rect = new Windows.Foundation.Rect(k * (Seg + Gap), 0, Seg, h) });
            return g;
        }
        bool firstVisible = !indeterminate && First != Colors.Transparent;
        pFirst.Data = Blocks(0, firstVisible ? i : 0);
        ((SolidColorBrush)pFirst.Fill).Color = First == Colors.Transparent ? Ui.Rule : First;
        pSecond.Data = Blocks(i, j);
        ((SolidColorBrush)pSecond.Fill).Color = Second;
        var empty = new GeometryGroup();
        for (int k = 0; k < n; k++)
            if ((k < i && !firstVisible) || k >= j)
                empty.Children.Add(new RectangleGeometry { Rect = new Windows.Foundation.Rect(k * (Seg + Gap), 0, Seg, h) });
        pEmpty.Data = empty;
    }
}

/// Memory as blocks, filling like something being poured in: other apps in
/// dim blocks, the model's share flowing toward a faint outline of where it
/// will end up, with a bright pulsing leading edge and a shimmer running
/// through what's already loaded. Values glide to their targets, so samples
/// a second apart still read as one continuous pour. When loading finishes
/// the model's blocks flash green and settle.
public sealed class PourBar : Grid
{
    const double Seg = 6, Gap = 2;
    readonly Microsoft.UI.Xaml.Shapes.Path pEmpty = new(), pApps = new(), pGhost = new(), pModel = new(), pShimmer = new(), pEdge = new();
    readonly DispatcherTimer timer = new() { Interval = TimeSpan.FromMilliseconds(33) };
    double apps, model, target, shown, shownApps;
    bool complete;
    DateTime started = DateTime.Now, last = DateTime.Now, completedAt;
    (int n, int ia, int im, int ig, int sh) painted = (-1, -1, -1, -1, -1);

    public PourBar(double height = 16)
    {
        Height = height;
        pEmpty.Fill = Ui.B(Ui.Rule); pApps.Fill = Ui.B(Ui.Dim); pGhost.Fill = Ui.B(Ui.C(0xb9a8ff, 60));
        pModel.Fill = Ui.B(Ui.Signal); pShimmer.Fill = Ui.B(Ui.Signal); pEdge.Fill = Ui.B(Colors.White);
        foreach (var x in new[] { pEmpty, pApps, pGhost, pModel, pShimmer, pEdge }) Children.Add(x);
        timer.Tick += (_, _) => Tick();
        Loaded += (_, _) => timer.Start();
        Unloaded += (_, _) => timer.Stop();
        SizeChanged += (_, _) => { painted = (-1, -1, -1, -1, -1); Paint(); };
    }

    /// Fractions of the whole bar: other apps, the model so far, the model when fully loaded.
    public void Set(double appsFrac, double modelFrac, double targetFrac)
    {
        apps = Clamp(appsFrac); target = Math.Max(Clamp(targetFrac), 0); model = Math.Min(Clamp(modelFrac), Math.Max(target, Clamp(modelFrac)));
        if (!timer.IsEnabled && IsLoaded) timer.Start();
    }

    public void Complete()
    {
        if (complete) return;
        complete = true; completedAt = DateTime.Now;
        model = Math.Max(model, target);
    }

    static double Clamp(double v) => double.IsFinite(v) ? Math.Clamp(v, 0, 1) : 0;

    void Tick()
    {
        var now = DateTime.Now;
        var dt = Math.Min(0.1, (now - last).TotalSeconds); last = now;
        // Glide toward the measured value, with a minimum pace so it visibly flows.
        var gap = model - shown;
        if (Math.Abs(gap) < 0.0005) shown = model;
        else shown += Math.Sign(gap) * Math.Max(Math.Abs(gap) * Math.Min(1, dt * 5), Math.Min(Math.Abs(gap), dt * 0.08));
        shownApps += (apps - shownApps) * Math.Min(1, dt * 6);
        Paint();
        if (complete && (now - completedAt).TotalSeconds > 1.6 && Math.Abs(shown - model) < 0.001) timer.Stop();
    }

    void Paint()
    {
        double w = ActualWidth, h = Height;
        if (w <= 0 || double.IsNaN(h)) return;
        int n = (int)((w + Gap) / (Seg + Gap));
        int Idx(double f) => Math.Clamp((int)Math.Round(f * n), 0, n);
        int ia = Idx(shownApps), im = Math.Max(ia, Idx(shownApps + shown)), ig = Math.Max(im, Idx(shownApps + Math.Max(target, shown)));
        var t = (DateTime.Now - started).TotalSeconds;
        int span = im - ia;
        int sh = !complete && span > 3 ? ia + (int)(t * 26 % (span + 8)) : -1; // shimmer position, with a pause between passes
        if (sh >= im) sh = -1;

        var key = (n, ia, im, ig, sh);
        if (key != painted)
        {
            painted = key;
            int edge = complete ? im : Math.Max(ia, im - 2);
            pApps.Data = Blocks(0, ia, h);
            pModel.Data = Blocks(ia, edge, h, sh);
            pShimmer.Data = sh >= 0 && sh < edge ? Blocks(sh, sh + 1, h) : new GeometryGroup();
            pEdge.Data = Blocks(edge, im, h);
            pGhost.Data = Blocks(im, ig, h);
            pEmpty.Data = Blocks(ig, n, h);
        }
        // Colors animate without touching geometry.
        var pulse = 0.5 + 0.5 * Math.Sin(t * 9);
        ((SolidColorBrush)pEdge.Fill).Color = Mix(Ui.Signal, Colors.White, 0.35 + 0.65 * pulse);
        ((SolidColorBrush)pShimmer.Fill).Color = Mix(Ui.Signal, Colors.White, 0.45);
        ((SolidColorBrush)pGhost.Fill).Color = Ui.C(0xb9a8ff, (byte)(38 + 34 * (0.5 + 0.5 * Math.Sin(t * 2.2))));
        if (complete)
        {
            var f = Math.Clamp((DateTime.Now - completedAt).TotalSeconds / 1.4, 0, 1);
            ((SolidColorBrush)pModel.Fill).Color = Mix(Ui.Ok, Ui.Signal, f * f);
            ((SolidColorBrush)pEdge.Fill).Color = Mix(Ui.Ok, Ui.Signal, f * f);
        }
    }

    static GeometryGroup Blocks(int from, int to, double h, int skip = -1)
    {
        var g = new GeometryGroup();
        for (int k = from; k < to; k++)
            if (k != skip) g.Children.Add(new RectangleGeometry { Rect = new Windows.Foundation.Rect(k * (Seg + Gap), 0, Seg, h) });
        return g;
    }

    static Color Mix(Color a, Color b, double f) => ColorHelper.FromArgb(255,
        (byte)(a.R + (b.R - a.R) * f), (byte)(a.G + (b.G - a.G) * f), (byte)(a.B + (b.B - a.B) * f));
}

/// Lays out its children left to right and wraps to a new line when the
/// next one doesn't fit (rows of buttons that must never run off the edge).
public sealed class FlowPanel : Panel
{
    public double Spacing { get; set; } = 10;
    public double LineSpacing { get; set; } = 10;

    protected override Windows.Foundation.Size MeasureOverride(Windows.Foundation.Size avail)
    {
        double x = 0, y = 0, line = 0, widest = 0;
        foreach (var c in Children)
        {
            c.Measure(new Windows.Foundation.Size(avail.Width, double.PositiveInfinity));
            var d = c.DesiredSize;
            if (x > 0 && x + d.Width > avail.Width) { y += line + LineSpacing; x = 0; line = 0; }
            x += d.Width + Spacing; line = Math.Max(line, d.Height); widest = Math.Max(widest, x - Spacing);
        }
        return new Windows.Foundation.Size(double.IsInfinity(avail.Width) ? widest : Math.Min(widest, avail.Width), y + line);
    }

    protected override Windows.Foundation.Size ArrangeOverride(Windows.Foundation.Size size)
    {
        double x = 0, y = 0, line = 0;
        foreach (var c in Children)
        {
            var d = c.DesiredSize;
            if (x > 0 && x + d.Width > size.Width) { y += line + LineSpacing; x = 0; line = 0; }
            c.Arrange(new Windows.Foundation.Rect(x, y, d.Width, d.Height));
            x += d.Width + Spacing; line = Math.Max(line, d.Height);
        }
        return size;
    }
}

/// Equal-width tiles: up to three across, two across below Narrow (about
/// 760 px), one below Narrow / 2. Tiles in a row share the tallest height.
public sealed class TilePanel : Panel
{
    public double Spacing { get; set; } = 12;
    public double Narrow { get; set; } = 760;
    double[] rows = Array.Empty<double>();
    int cols = 1;

    int Cols(double w) => Math.Max(1, Math.Min(Children.Count, double.IsInfinity(w) || w >= Narrow ? 3 : w >= Narrow / 2 ? 2 : 1));

    protected override Windows.Foundation.Size MeasureOverride(Windows.Foundation.Size avail)
    {
        double w = double.IsInfinity(avail.Width) ? 3 * 260 + 2 * Spacing : avail.Width;
        cols = Cols(w);
        double cw = (w - Spacing * (cols - 1)) / cols;
        rows = new double[(Children.Count + cols - 1) / cols];
        for (int i = 0; i < Children.Count; i++)
        {
            Children[i].Measure(new Windows.Foundation.Size(cw, double.PositiveInfinity));
            rows[i / cols] = Math.Max(rows[i / cols], Children[i].DesiredSize.Height);
        }
        return new Windows.Foundation.Size(w, rows.Sum() + Spacing * Math.Max(0, rows.Length - 1));
    }

    protected override Windows.Foundation.Size ArrangeOverride(Windows.Foundation.Size size)
    {
        double cw = (size.Width - Spacing * (cols - 1)) / cols, y = 0;
        for (int i = 0; i < Children.Count; i++)
        {
            int r = i / cols, c = i % cols;
            if (c == 0 && r > 0) y += rows[r - 1] + Spacing;
            Children[i].Arrange(new Windows.Foundation.Rect(c * (cw + Spacing), y, cw, r < rows.Length ? rows[r] : 0));
        }
        return size;
    }
}

public sealed class Spinner : Grid
{
    static readonly string[] Frames = { "⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏" };
    readonly DispatcherTimer timer = new() { Interval = TimeSpan.FromMilliseconds(80) };
    readonly TextBlock t;
    int i;
    public Spinner(Color? c = null)
    {
        t = new TextBlock { FontFamily = Ui.Mono, FontSize = 13, Foreground = Ui.B(c ?? Ui.Signal), Text = Frames[0] };
        Children.Add(t);
        timer.Tick += (_, _) => t.Text = Frames[++i % Frames.Length];
        Loaded += (_, _) => timer.Start();
        Unloaded += (_, _) => timer.Stop();
    }
}

/// The two-tone block logo: "EZ" dim, "LM" bright; glows lilac while busy.
public sealed class Logo : StackPanel
{
    static readonly string[] E = { "███████╗", "██╔════╝", "█████╗  ", "██╔══╝  ", "███████╗", "╚══════╝" };
    static readonly string[] Z = { "███████╗", "╚══███╔╝", "  ███╔╝ ", " ███╔╝  ", "███████╗", "╚══════╝" };
    static readonly string[] L = { "██╗     ", "██║     ", "██║     ", "██║     ", "███████╗", "╚══════╝" };
    static readonly string[] M = { "███╗   ███╗", "████╗ ████║", "██╔████╔██║", "██║╚██╔╝██║", "██║ ╚═╝ ██║", "╚═╝     ╚═╝" };
    readonly List<Run> lm = new();
    readonly DispatcherTimer timer = new() { Interval = TimeSpan.FromMilliseconds(33) };
    bool busy;
    DateTime flashUntil;

    public Logo(double size = 13)
    {
        for (int i = 0; i < 6; i++)
        {
            var t = new TextBlock { FontFamily = Ui.Mono, FontSize = size, FontWeight = FontWeights.Bold, LineHeight = size * 1.05, LineStackingStrategy = LineStackingStrategy.BlockLineHeight };
            t.Inlines.Add(new Run { Text = E[i] + Z[i], Foreground = Ui.B(Ui.Dim) });
            var r = new Run { Text = L[i] + M[i], Foreground = Ui.B(Ui.Ink) };
            t.Inlines.Add(r); lm.Add(r);
            Children.Add(t);
        }
        timer.Tick += (_, _) => Tick();
        timer.Start();
        Unloaded += (_, _) => timer.Stop();
    }

    public void SetBusy(bool on) => busy = on;
    public void Flash() => flashUntil = DateTime.Now.AddSeconds(1.2);

    void Tick()
    {
        Color c = Ui.Ink;
        if (DateTime.Now < flashUntil) c = Ui.Ok;
        else if (busy)
        {
            var p = (Math.Sin(DateTime.Now.TimeOfDay.TotalSeconds * 3.4) + 1) / 2;
            c = ColorHelper.FromArgb(255, (byte)(Ui.Ink.R + (Ui.Signal.R - Ui.Ink.R) * p), (byte)(Ui.Ink.G + (Ui.Signal.G - Ui.Ink.G) * p), (byte)(Ui.Ink.B + (Ui.Signal.B - Ui.Ink.B) * p));
        }
        foreach (var r in lm) ((SolidColorBrush)r.Foreground).Color = c;
    }
}

/// Typewriter text that cancels itself when retargeted.
public static class Typer
{
    public static async void Type(TextBlock t, string text, int ms = 30)
    {
        var id = Guid.NewGuid();
        t.Tag = id;
        t.Text = "";
        for (int i = 1; i <= text.Length; i++)
        {
            if (!Equals(t.Tag, id)) return;
            t.Text = text[..i] + (i < text.Length ? "▍" : "");
            await Task.Delay(ms);
        }
    }
}

/// Real vendor logos (bundled PNGs) and pixel-art flags.
public static class Marks
{
    static readonly (string key, string file)[] Table =
    {
        ("qwen","qwen"),("alibaba","qwen"),("openai","openai"),("google","google"),("gemma","google"),("deepseek","deepseek"),
        ("zhipu","zhipu"),("z.ai","zhipu"),("nvidia","nvidia"),("mistral","mistral"),("meta","meta"),("microsoft","microsoft"),
        ("ibm","ibm"),("moonshot","moonshot"),("kimi","moonshot"),("minimax","minimax"),("nous","nous"),("tencent","tencent"),
        ("xiaomi","xiaomi"),("liquid","liquid"),("prism","prismml"),("nanbeige","nanbeige"),("kanzhun","nanbeige"),
    };

    public static UIElement Logo(string? vendor, double size = 20)
    {
        var v = (vendor ?? "").ToLowerInvariant();
        var file = Table.FirstOrDefault(t => v.Contains(t.key)).file;
        var path = file == null ? null : System.IO.Path.Combine(AppContext.BaseDirectory, "Assets", "Logos", file + ".png");
        if (path != null && File.Exists(path))
        {
            if (!logoCache.TryGetValue(path, out var src)) logoCache[path] = src = new BitmapImage(new Uri(path));
            return new Image { Source = src, Width = size, Height = size, Stretch = Stretch.Uniform };
        }
        return new Border { Width = size, Height = size, BorderBrush = Ui.B(Ui.Faint), BorderThickness = new Thickness(1),
            Child = new TextBlock { Text = string.IsNullOrEmpty(vendor) ? "?" : vendor[..1], FontFamily = Ui.Mono, FontSize = size * 0.55, Foreground = Ui.B(Ui.Dim), HorizontalAlignment = HorizontalAlignment.Center, VerticalAlignment = VerticalAlignment.Center } };
    }

    static readonly Dictionary<string, string> Names = new()
    {
        ["CN"] = "China", ["US"] = "United States", ["FR"] = "France", ["GB"] = "United Kingdom", ["AE"] = "UAE", ["CA"] = "Canada",
        ["JP"] = "Japan", ["KR"] = "South Korea", ["DE"] = "Germany", ["IL"] = "Israel", ["IN"] = "India", ["CH"] = "Switzerland",
    };

    static (string[] rows, Dictionary<char, Color> pal)? Art(string code)
    {
        Color red = Ui.C(0xde2929), yellow = Ui.C(0xffd600), blue = Ui.C(0x00389e), white = Ui.C(0xf2f2f2), green = Ui.C(0x008c45), black = Ui.C(0x141414), gold = Ui.C(0xffcc00);
        return code.ToUpperInvariant() switch
        {
            "CN" => (new[] { "rrrrrrrrrrrrrrrr", "rryrrryrrrrrrrrr", "ryyyrrrrrrrrrrrr", "yyyyyryrrrrrrrrr", "ryyyrrrrrrrrrrrr", "ryryrryrrrrrrrrr", "rrrrrrrrrrrrrrrr", "rrrrrrrrrrrrrrrr", "rrrrrrrrrrrrrrrr", "rrrrrrrrrrrrrrrr" }, new() { ['r'] = red, ['y'] = yellow }),
            "US" => (new[] { "bbbbbbbrrrrrrrrr", "bwbwbwbwwwwwwwww", "bbbbbbbrrrrrrrrr", "bwbwbwbwwwwwwwww", "bbbbbbbrrrrrrrrr", "wwwwwwwwwwwwwwww", "rrrrrrrrrrrrrrrr", "wwwwwwwwwwwwwwww", "rrrrrrrrrrrrrrrr", "wwwwwwwwwwwwwwww" }, new() { ['b'] = blue, ['r'] = red, ['w'] = white }),
            "FR" => (Enumerable.Repeat("bbbbbwwwwwwrrrrr", 10).ToArray(), new() { ['b'] = blue, ['w'] = white, ['r'] = red }),
            "DE" => (Enumerable.Repeat("kkkkkkkkkkkkkkkk", 3).Concat(Enumerable.Repeat("rrrrrrrrrrrrrrrr", 4)).Concat(Enumerable.Repeat("gggggggggggggggg", 3)).ToArray(), new() { ['k'] = black, ['r'] = red, ['g'] = gold }),
            "GB" => (new[] { "wbbbbbbrrbbbbbbw", "bwbbbbbrrbbbbbwb", "bbwbbbbrrbbbbwbb", "wwwwwwwrrwwwwwww", "rrrrrrrrrrrrrrrr", "rrrrrrrrrrrrrrrr", "wwwwwwwrrwwwwwww", "bbwbbbbrrbbbbwbb", "bwbbbbbrrbbbbbwb", "wbbbbbbrrbbbbbbw" }, new() { ['b'] = blue, ['r'] = red, ['w'] = white }),
            "JP" => (new[] { "wwwwwwwwwwwwwwww", "wwwwwwwwwwwwwwww", "wwwwwwrrrrwwwwww", "wwwwwrrrrrrwwwww", "wwwwwrrrrrrwwwww", "wwwwwrrrrrrwwwww", "wwwwwrrrrrrwwwww", "wwwwwwrrrrwwwwww", "wwwwwwwwwwwwwwww", "wwwwwwwwwwwwwwww" }, new() { ['w'] = white, ['r'] = red }),
            "CA" => (new[] { "rrrrwwwwwwwwrrrr", "rrrrwwwwrwwwrrrr", "rrrrwwwrrrwwrrrr", "rrrrwrwrrrwrrrrr", "rrrrwrrrrrrrrrrr", "rrrrwwrrrrrwrrrr", "rrrrwwwrrrwwrrrr", "rrrrwwwwrwwwrrrr", "rrrrwwwwrwwwrrrr", "rrrrwwwwwwwwrrrr" }, new() { ['r'] = red, ['w'] = white }),
            "AE" => (new[] { "rrrrgggggggggggg", "rrrrgggggggggggg", "rrrrgggggggggggg", "rrrrwwwwwwwwwwww", "rrrrwwwwwwwwwwww", "rrrrwwwwwwwwwwww", "rrrrwwwwwwwwwwww", "rrrrkkkkkkkkkkkk", "rrrrkkkkkkkkkkkk", "rrrrkkkkkkkkkkkk" }, new() { ['r'] = red, ['g'] = green, ['w'] = white, ['k'] = black }),
            _ => null,
        };
    }

    static readonly Dictionary<string, WriteableBitmap?> flagCache = new();

    /// Pixel-art flag rendered once per country into a bitmap (4 px per cell)
    /// and shared by every Image that shows it.
    static WriteableBitmap? FlagImage(string code)
    {
        code = code.ToUpperInvariant();
        if (flagCache.TryGetValue(code, out var hit)) return hit;
        WriteableBitmap? bmp = null;
        if (Art(code) is { } a)
        {
            const int px = 4, w = 16 * px, h = 10 * px;
            var buf = new byte[w * h * 4];
            for (int y = 0; y < h; y++)
                for (int x = 0; x < w; x++)
                {
                    var row = a.rows[Math.Min(y / px, a.rows.Length - 1)];
                    if (!a.pal.TryGetValue(row[Math.Min(x / px, row.Length - 1)], out var c)) continue;
                    int o = (y * w + x) * 4;
                    buf[o] = c.B; buf[o + 1] = c.G; buf[o + 2] = c.R; buf[o + 3] = 255;
                }
            bmp = new WriteableBitmap(w, h);
            using (var st = System.Runtime.InteropServices.WindowsRuntime.WindowsRuntimeBufferExtensions.AsStream(bmp.PixelBuffer)) st.Write(buf, 0, buf.Length);
            bmp.Invalidate();
        }
        flagCache[code] = bmp;
        return bmp;
    }

    static readonly Dictionary<string, BitmapImage> logoCache = new();

    /// The display scale logos are decoded for (Shell keeps it in step with
    /// XamlRoot.RasterizationScale); Override pins it for snapshots.
    public static double Scale = 1;
    public static double? ScaleOverride;

    static string AppLogoPath(string? icon) => System.IO.Path.Combine(AppContext.BaseDirectory, "Assets", "Logos", $"app-{icon}.png");

    public static bool HasAppLogo(string? icon) => !string.IsNullOrEmpty(icon) && File.Exists(AppLogoPath(icon));

    /// An app's own logo (app-<icon>.png) in a size x size box: 36 on Connect
    /// tiles, 16 on launch buttons, 20 in Overview rows. Decoded at the
    /// display's pixel size, so it stays sharp at 150% without a 512 px decode
    /// per copy. The DeepSeek Harness whale is a bare glyph: drawn at 80 % so
    /// its weight matches the tiled logos. Hidden from screen readers (the
    /// name sits next to it). Without the file: the pixel wordmark at tile
    /// size, nothing smaller.
    public static UIElement AppLogo(string? icon, double size)
    {
        if (!HasAppLogo(icon)) return size >= 30 ? AppMark(icon ?? "") : new Border { Width = 0, Visibility = Visibility.Collapsed };
        var path = AppLogoPath(icon);
        double inner = icon == "dsh" ? Math.Round(size * 0.8) : size;
        var img = new Image { Width = inner, Height = inner, Stretch = Stretch.Uniform, HorizontalAlignment = HorizontalAlignment.Center, VerticalAlignment = VerticalAlignment.Center };
        void Load(double scale)
        {
            int px = (int)Math.Ceiling(inner * scale);
            var key = $"{path}@{px}";
            if (!logoCache.TryGetValue(key, out var src)) logoCache[key] = src = new BitmapImage { DecodePixelWidth = px, DecodePixelType = DecodePixelType.Physical, UriSource = new Uri(path) };
            img.Source = src;
            img.Tag = scale;
        }
        Load(ScaleOverride ?? Scale);
        img.Loaded += (_, _) => { var s = ScaleOverride ?? img.XamlRoot?.RasterizationScale ?? Scale; if (!Equals(img.Tag, s)) Load(s); };
        var box = new Border { Width = size, Height = size, Child = img, VerticalAlignment = VerticalAlignment.Center };
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetAccessibilityView(box, Microsoft.UI.Xaml.Automation.Peers.AccessibilityView.Raw);
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetAccessibilityView(img, Microsoft.UI.Xaml.Automation.Peers.AccessibilityView.Raw);
        return box;
    }

    /// The half-block pixel wordmark (same as the Mac app), switched on id:
    /// "open" dim + "code" bright, "hermes" in gold, "dsh" in white.
    public static UIElement AppMark(string id)
    {
        var t = new TextBlock { FontFamily = Ui.Mono, FontSize = 8, FontWeight = FontWeights.Bold, LineHeight = 8, LineStackingStrategy = LineStackingStrategy.BlockLineHeight, Margin = new Thickness(0, 0, 0, 6) };
        var parts = AppText.Wordmark(id);
        if (parts.Length == 0) { var n = Ui.Txt(id, 12, Ui.Dim, true); n.Margin = new Thickness(0, 0, 0, 6); return n; }
        Color bright = id == "hermes" ? Ui.Gold : id == "dsh" ? Colors.White : Ui.Ink;
        var lines = parts.Select(p => (rows: PixelFont.Lines(p.Word), p.Bright)).ToArray();
        for (int i = 0; i < 3; i++)
            for (int k = 0; k < lines.Length; k++)
                t.Inlines.Add(new Run { Text = lines[k].rows[i] + (i < 2 && k == lines.Length - 1 ? "\n" : ""), Foreground = Ui.B(lines[k].Bright ? bright : Ui.Dim) });
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetAccessibilityView(t, Microsoft.UI.Xaml.Automation.Peers.AccessibilityView.Raw);
        return t;
    }

    public static UIElement Origin(string? code)
    {
        var s = Ui.Row(6);
        if (string.IsNullOrEmpty(code)) return s;
        if (FlagImage(code) is { } img)
            s.Children.Add(new Image { Source = img, Width = 24, Height = 15, Stretch = Stretch.Fill, VerticalAlignment = VerticalAlignment.Center });
        var name = Names.TryGetValue(code.ToUpperInvariant(), out var n) ? n : code.ToUpperInvariant();
        var t = Ui.Txt(name, 10, Ui.Dim); t.VerticalAlignment = VerticalAlignment.Center;
        s.Children.Add(t);
        return s;
    }
}

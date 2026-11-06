using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;
using Windows.UI;

namespace EZLM;

/// What the model is doing, as the signal shows it.
public enum Mood { Off, Loading, Idle, Reading, Thinking, Writing, Error }

/// The signal's motion, kept outside the control so the wave carries on
/// smoothly when the Chat page is rebuilt.
public sealed class WaveState
{
    public double Phase, Energy, Kick, Ripple = -1, T;
    /// How much of the line is drawn: it sweeps in once, the first time Chat opens.
    public double Drawn;
    public Color Color = Ui.Faint;
}

/// The signal: a braille waveform across the top of Chat that shows what the
/// model is doing, whichever app is using it. It breathes while idle, scans
/// while reading a prompt, beats while thinking, pulses with the words it
/// writes, and lies flat (with a heartbeat blip) when the server is off.
public sealed class SignalWave : Grid
{
    public Func<Mood> MoodOf = () => Mood.Idle;
    public Func<double> RateOf = () => 0; // tokens a second while writing
    /// About 30 times a second while the wave is on screen.
    public event Action? Frame;

    const int Rows = 4; // braille rows, 4 dots each: 16 dots tall
    const double Size = 15;
    readonly WaveState s;
    readonly TextBlock back, front;
    readonly LinearGradientBrush backBrush, frontBrush;
    readonly DispatcherTimer timer = new() { Interval = TimeSpan.FromMilliseconds(33) };
    int cols = 90;
    double charW = 7.2;

    public SignalWave(WaveState state)
    {
        s = state;
        Height = Rows * Size + 2;
        backBrush = Gradient(); frontBrush = Gradient();
        back = Layer(backBrush); front = Layer(frontBrush);
        timer.Tick += (_, _) => Step();
        Loaded += (_, _) => { MeasureCells(); timer.Start(); };
        Unloaded += (_, _) => timer.Stop();
        SizeChanged += (_, e) => cols = Math.Clamp((int)(e.NewSize.Width / charW), 16, 400);
        IsHitTestVisible = false;
    }

    /// A burst of energy: every chunk of words written kicks the wave a little.
    public void Pulse(double amount) => s.Kick = Math.Min(1.6, s.Kick + amount);

    /// One ripple from the middle: an answer just finished.
    public void Ripple() => s.Ripple = 0;

    static LinearGradientBrush Gradient()
    {
        var b = new LinearGradientBrush { StartPoint = new Windows.Foundation.Point(0, 0.5), EndPoint = new Windows.Foundation.Point(1, 0.5) };
        foreach (var o in new[] { 0, 0.22, 0.5, 0.78, 1 }) b.GradientStops.Add(new GradientStop { Offset = o, Color = Ui.Rule });
        return b;
    }

    TextBlock Layer(Brush brush)
    {
        var t = new TextBlock
        {
            FontFamily = Ui.Mono, FontSize = Size, LineHeight = Size, LineStackingStrategy = LineStackingStrategy.BlockLineHeight,
            TextWrapping = TextWrapping.NoWrap, Foreground = brush, IsHitTestVisible = false,
        };
        Children.Add(t);
        return t;
    }

    void MeasureCells()
    {
        // Braille cells are as wide as any glyph of the font: measure one run of them.
        var probe = new TextBlock { FontFamily = Ui.Mono, FontSize = Size, Text = new string('⣿', 40) };
        probe.Measure(new Windows.Foundation.Size(double.PositiveInfinity, double.PositiveInfinity));
        if (probe.DesiredSize.Width > 0) charW = probe.DesiredSize.Width / 40;
        if (ActualWidth > 0) cols = Math.Clamp((int)(ActualWidth / charW), 16, 400);
    }

    void Step()
    {
        const double dt = 0.033;
        var mood = MoodOf();
        var rate = Math.Max(0, RateOf());
        var (energy, speed, color) = mood switch
        {
            Mood.Off => (0.05, 0.5, Ui.Dim),
            Mood.Loading => (0.32, 1.0, Ui.Gold),
            Mood.Reading => (0.62, 3.2, Ui.Signal),
            Mood.Thinking => (0.72, 2.3, Ui.Gold),
            Mood.Writing => (0.5 + Math.Min(0.3, rate / 160), 2.0 + Math.Min(3, rate / 35), Lerp(Ui.Signal, Ui.Ink, 0.25)),
            Mood.Error => (0.04, 0.3, Ui.Bad),
            _ => (0.42 + 0.14 * Math.Sin(s.T * 0.9), 0.8, Lerp(Ui.Signal, Ui.Faint, 0.2)), // idle: a slow breath
        };
        s.Energy += (energy + s.Kick * 0.35 - s.Energy) * 0.07;
        s.Kick *= 0.9;
        s.Phase += speed * dt;
        s.T += dt;
        if (s.Ripple >= 0 && (s.Ripple += dt) > 1.6) s.Ripple = -1;
        s.Color = Lerp(s.Color, color, 0.08);
        s.Drawn = Math.Min(1, s.Drawn + dt / 0.9);
        Paint(frontBrush, s.Color, 1);
        Paint(backBrush, s.Color, 0.5);

        int w = cols * 2, h = Rows * 4;
        front.Text = Render(w, h, u => Wave(mood, u, 0), 2);
        back.Text = Render(w, h, u => -Wave(mood, u, 1) * 0.8, 1);
        Frame?.Invoke();
    }

    /// The wave's height at u (0..1 across), from -1 to 1. Layer 1 is the
    /// fainter echo behind: slower, shifted, longer.
    double Wave(Mood mood, double u, int layer)
    {
        double p = s.Phase * (layer == 0 ? 1 : 0.7) + layer * 1.9, k = layer == 0 ? 1 : 0.8;
        double env = Math.Pow(Math.Sin(Math.PI * u), 0.8); // meets the center line at both ends
        double f = mood switch
        {
            Mood.Off or Mood.Error => Blip(u, layer),
            Mood.Reading => Math.Sin(Tau * 6 * k * u - 3 * p) * (0.22 + 0.78 * Packet(u, layer)),
            Mood.Thinking => 0.55 * (Math.Sin(Tau * 4 * k * u - p) + Math.Sin(Tau * 4.45 * k * u - 1.32 * p)),
            Mood.Writing => 0.68 * Math.Sin(Tau * 2.2 * k * u - p) + 0.32 * Math.Sin(Tau * 5.3 * k * u - 1.9 * p) * (0.6 + s.Kick),
            Mood.Loading => Math.Sin(Tau * 1.4 * k * u - p) * (0.35 + 0.65 * Packet(u, layer)),
            _ => 0.72 * Math.Sin(Tau * 1.5 * k * u - p) + 0.28 * Math.Sin(Tau * 3.7 * k * u + 0.7 * p),
        };
        if (s.Ripple >= 0)
        {
            double d = Math.Abs(u - 0.5);
            f += 0.9 * Math.Exp(-s.Ripple * 2.6) * Math.Exp(-d * 3) * Math.Sin(Tau * 9 * d - s.Ripple * 14);
        }
        return Math.Clamp(s.Energy * env * f, -1, 1);
    }

    // A bright packet that sweeps across, again and again (reading, loading).
    double Packet(double u, int layer)
    {
        double at = (s.T * (layer == 0 ? 0.55 : 0.4) + layer * 0.5) % 1.3 - 0.15;
        return Math.Exp(-Math.Pow((u - at) / 0.09, 2));
    }

    // Flat, with a small heartbeat now and then: the server is off but EZLM is here.
    double Blip(double u, int layer)
    {
        if (layer == 1) return 0;
        double at = (s.T / 4.0 % 1) * 1.4 - 0.2, d = u - at;
        return Math.Abs(d) > 0.04 ? 0 : 1.6 * Math.Exp(-Math.Pow(d / 0.012, 2)) * Math.Sin(d * 260);
    }

    const double Tau = Math.PI * 2;
    // Braille dot bits by [row in cell, column in cell].
    static readonly int[,] Bit = { { 0x01, 0x08 }, { 0x02, 0x10 }, { 0x04, 0x20 }, { 0x40, 0x80 } };

    /// Draws y(u) as a connected line of braille dots, w by h dots.
    string Render(int w, int h, Func<double, double> y, int thick)
    {
        var cells = new int[Rows, cols];
        double mid = (h - 1) / 2.0;
        int prev = -1, upto = (int)(w * (1 - Math.Pow(1 - s.Drawn, 3))); // ease-out sweep
        for (int x = 0; x < upto; x++)
        {
            int yi = Math.Clamp((int)Math.Round(mid - y(x / (double)(w - 1)) * mid), 0, h - 1);
            int lo = prev < 0 ? yi : Math.Min(prev, yi), hi = prev < 0 ? yi : Math.Max(prev, yi);
            for (int yy = lo; yy <= Math.Min(h - 1, hi + thick - 1); yy++) cells[yy / 4, x / 2] |= Bit[yy % 4, x % 2];
            prev = yi;
        }
        var sb = new System.Text.StringBuilder(Rows * (cols + 1));
        for (int r = 0; r < Rows; r++)
        {
            if (r > 0) sb.Append('\n');
            for (int c = 0; c < cols; c++) sb.Append((char)(0x2800 + cells[r, c]));
        }
        return sb.ToString();
    }

    static void Paint(LinearGradientBrush b, Color c, double strength)
    {
        var edge = Lerp(Ui.Rule, c, 0.18);
        var stops = b.GradientStops;
        stops[0].Color = edge;
        stops[1].Color = Lerp(edge, c, 0.6 * strength);
        stops[2].Color = Lerp(edge, c, strength);
        stops[3].Color = Lerp(edge, c, 0.6 * strength);
        stops[4].Color = edge;
    }

    public static Color Lerp(Color a, Color b, double t) => Microsoft.UI.ColorHelper.FromArgb(
        (byte)(a.A + (b.A - a.A) * t), (byte)(a.R + (b.R - a.R) * t), (byte)(a.G + (b.G - a.G) * t), (byte)(a.B + (b.B - a.B) * t));
}

using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using static EZLM.Ui;

namespace EZLM;

/// The "pouring the model into memory" panel shown while a model loads.
///
/// It's driven by real numbers: graphics memory from the driver as the server
/// uploads the model, system memory from how much of the file has been read
/// in (the pre-read), and each bar's outline is the planned size. The lowest
/// usage seen since loading began counts as "other apps", which also handles
/// a switch where the previous model is unloaded first.
sealed class MemoryFillView
{
    public readonly StackPanel Root;
    readonly TextBlock title = Txt("", 13, Ink, true, false), total = Txt("", 12, Dim, false, false);
    readonly StackPanel rows = Col(12);
    Row? gpu, ram;
    double warm; // fraction of the model file read into memory so far
    bool done;

    sealed class Row
    {
        public readonly StackPanel Root;
        public readonly PourBar Bar = new(16);
        readonly TextBlock nums = Txt("", 12, Dim, false, false);
        public ulong Total, Need, Model;
        public ulong? Base;

        public Row(string label)
        {
            var head = new Grid();
            head.Children.Add(Txt(label, 12, Dim));
            nums.HorizontalAlignment = HorizontalAlignment.Right;
            head.Children.Add(nums);
            Root = Col(6, head, Bar);
        }

        public void Sample(ulong used, double warm, bool done)
        {
            Base = Base is { } b ? Math.Min(b, used) : used;
            var measured = used > Base ? used - Base.Value : 0;
            var target = Need > 0 ? Need : Math.Max(Model, measured);
            Model = Math.Max(Model, Math.Min(Math.Max(measured, (ulong)(warm * Need)), target));
            if (done) Model = target;
            double T = Math.Max(Total, 1);
            Bar.Set(Base.Value / T, Model / T, target / T);
            if (done) Bar.Complete();
            nums.Text = done ? $"{Fmt.Gb(target)} loaded" : Need > 0 ? $"{Fmt.Gb(Model)} of {Fmt.Gb(Need)}" : $"{Fmt.Gb(Model)}";
        }
    }

    public MemoryFillView(string modelName)
    {
        title.Text = $"Pouring {modelName} into memory";
        var head = new Grid();
        head.Children.Add(title);
        total.HorizontalAlignment = HorizontalAlignment.Right;
        head.Children.Add(total);
        Root = Col(12, head, rows);
        Root.Padding = new Thickness(16, 14, 16, 16);
        Root.BorderBrush = B(Rule); Root.BorderThickness = new Thickness(1);
    }

    /// The pre-read: how much of the model file is in memory now.
    public void Warm(ulong doneBytes, ulong totalBytes)
    {
        if (totalBytes > 0) warm = Math.Max(warm, (double)doneBytes / totalBytes);
    }

    public void Feed(Live l, ulong gpuNeed = 0, ulong ramNeed = 0)
    {
        if (l.LoadTotal > 0) Warm(l.LoadDone, l.LoadTotal);
        if (gpuNeed == 0) gpuNeed = l.GpuPlan;
        if (ramNeed == 0) ramNeed = l.RamPlan;
        if (l.Unified)
        {
            gpu ??= AddRow("Memory, shared with graphics");
            gpu.Total = l.RamTotal; gpu.Need = gpuNeed + ramNeed;
            gpu.Sample(l.RamTotal - Math.Min(l.RamAvail, l.RamTotal), warm, done);
        }
        else
        {
            if (l.GpuTotal > 0)
            {
                gpu ??= AddRow("Graphics memory");
                gpu.Total = l.GpuTotal; gpu.Need = gpuNeed;
                gpu.Sample(l.GpuTotal - Math.Min(l.GpuFree, l.GpuTotal), 0, done);
            }
            // System memory matters when part of the model lives there (or there's no graphics card).
            if (ramNeed >= (1UL << 30) || l.GpuTotal == 0)
            {
                ram ??= AddRow("System memory");
                ram.Total = l.RamTotal; ram.Need = ramNeed;
                ram.Sample(l.RamTotal - Math.Min(l.RamAvail, l.RamTotal), warm, done);
            }
        }
        ulong loaded = (gpu?.Model ?? 0) + (ram?.Model ?? 0), need = (gpu?.Need ?? 0) + (ram?.Need ?? 0);
        total.Text = need > 0 ? $"{Math.Min(100, (int)(100.0 * loaded / need))}%" : "";
    }

    /// Loading finished: fill to the planned size and flash.
    public void Complete(Live? last = null)
    {
        if (done) return;
        done = true;
        title.Text = title.Text.Replace("Pouring", "Loaded").Replace(" into memory", "");
        title.Foreground = B(Ok);
        total.Text = "100%";
        if (last != null) Feed(last);
        else { gpu?.Sample((gpu.Base ?? 0) + gpu.Model, 0, true); ram?.Sample((ram.Base ?? 0) + ram.Model, 1, true); }
    }

    Row AddRow(string label)
    {
        var r = new Row(label);
        rows.Children.Add(r.Root);
        return r;
    }
}

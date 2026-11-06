using System.Text.Json;

namespace EZLM;

// Mirrors of the engine's JSON (internal/app, internal/fit, internal/scan).
// Everything is lenient so a newer engine never breaks an older app.

public class Gpu { public string Name { get; set; } = ""; public string Vendor { get; set; } = ""; public ulong MemTotal { get; set; } public ulong MemFree { get; set; } public string? Driver { get; set; } }
public class Proc { public int Pid { get; set; } public string Name { get; set; } = ""; public ulong Mem { get; set; } public bool Killable { get; set; } }

public class Hardware
{
    public string Os { get; set; } = "";
    public string? OsName { get; set; }
    public string Arch { get; set; } = "";
    public string CpuName { get; set; } = "";
    public int Cores { get; set; }
    public ulong RamTotal { get; set; }
    public ulong RamAvail { get; set; }
    public ulong DiskFree { get; set; }
    public string? ModelsDir { get; set; }
    public List<Gpu>? Gpus { get; set; }
    public string Backend { get; set; } = "";
    public bool Unified { get; set; }
    public ulong GpuUsable { get; set; }
    public List<Proc>? GpuProcs { get; set; }
    public List<Proc>? TopMem { get; set; }
    public List<string>? Notes { get; set; }
    public ulong GpuTotal => (ulong)(Gpus ?? new()).Sum(g => (double)g.MemTotal);
    public ulong GpuFree => (ulong)(Gpus ?? new()).Sum(g => (double)g.MemFree);
    public string MachineWord => Os == "darwin" ? "Mac" : Os == "windows" ? "PC" : "computer";
}

public class Model
{
    public string Id { get; set; } = "";
    public string Name { get; set; } = "";
    public string? Vendor { get; set; }
    public string? Country { get; set; }
    public string? Repo { get; set; }
    public double ParamsB { get; set; }
    public double ActiveParamsB { get; set; }
    public bool Moe { get; set; }
    public double Score { get; set; }
    public int Layers { get; set; }
    public bool ToolCalling { get; set; }
}

public class Quant { public string Name { get; set; } = ""; public ulong Bytes { get; set; } public string? Repo { get; set; } public bool Mtp { get; set; } }

public class Plan
{
    public Model Model { get; set; } = new();
    public Quant Quant { get; set; } = new();
    public int Ctx { get; set; }
    public string? Kv { get; set; }
    public string Mode { get; set; } = "";
    public int Ngl { get; set; }
    public int NCpuMoe { get; set; }
    public string? Spec { get; set; }
    public ulong GpuNeed { get; set; }
    public ulong RamNeed { get; set; }
    public ulong Download { get; set; }
    public double TokPerSec { get; set; }
    public string Speed { get; set; } = "";
    public double Effective { get; set; }
    public bool DiskOk { get; set; }
    public bool Downloaded { get; set; }
    public ulong Partial { get; set; }
    public int Words => Math.Max(1, (int)Math.Round(TokPerSec * 0.75));
    /// How much it can keep in mind, and whether it works with coding agents: a card's at-a-glance facts.
    public string CtxLine => $"{Ctx / 1024}k context" + (Model.ToolCalling ? ", works with coding agents" : ", chat only");
    public string SpeedWord => Speed switch { "fast" => "Fast", "good" => "Comfortable", "slow" => "Slow", _ => "" };

    public string QuantLine
    {
        get
        {
            var who = (Quant.Repo ?? Model.Repo ?? "").Split('/').FirstOrDefault() ?? "";
            var s = Quant.Name.Replace("GSQ-RCO-", "GSQ ") + (who != "" ? $" by {who}" : "");
            if (!string.IsNullOrEmpty(Spec)) s += Spec == "draft-mtp" ? ", MTP speed-up" : ", draft model speed-up";
            if (!string.IsNullOrEmpty(Kv) && Kv != "f16") s += $", {Kv} cache";
            return s;
        }
    }

    public string How(bool unified)
    {
        var kind = Model.Moe ? $"Mixture of experts, {Math.Round(Model.ParamsB)}B total, {Math.Round(Model.ActiveParamsB)}B per word" : $"Dense, all {Math.Round(Model.ParamsB)}B per word";
        var where = Mode switch
        {
            "gpu" => unified ? "fits in shared memory" : "all on the graphics card",
            "hybrid" => $"experts in system memory ({NCpuMoe} of {Model.Layers} layers)",
            "split" => $"{Ngl} of {Model.Layers} layers on the graphics card",
            _ => "runs on the processor",
        };
        return $"{kind}; {where}";
    }
}

public class ScanResult
{
    public string? Pref { get; set; }
    public Hardware Hw { get; set; } = new();
    public List<Plan>? Plans { get; set; }
    public Plan? Recommended { get; set; }
    public Plan? Unlock { get; set; }
    public double UnlockFreeGb { get; set; }
    public bool UnlockGpu { get; set; }
}

public class ApiKey { public string Id { get; set; } = ""; public string Name { get; set; } = ""; public string Key { get; set; } = ""; }

public class ServerConfig
{
    public string Key { get; set; } = "";
    public List<ApiKey>? Keys { get; set; }
    public int Port { get; set; }
    public bool Lan { get; set; }
    public string ModelId { get; set; } = "";
    public string ModelName { get; set; } = "";
    public string Quant { get; set; } = "";
    public int Ctx { get; set; }
    public string? CpuFallback { get; set; } // why it runs on the processor (the graphics card couldn't load it)
}

/// An app EZLM connects to (OpenCode, Hermes, DeepSeek Harness). Everything
/// after Connected is optional: older engines don't send it.
public class Target
{
    public string Id { get; set; } = "";
    public string Name { get; set; } = "";
    public string? Desc { get; set; }
    public bool Supported { get; set; }
    public string? Why { get; set; }
    public bool Installed { get; set; }
    public bool Connected { get; set; }
    public string? Icon { get; set; }
    public string? Version { get; set; }
    public string? Bin { get; set; }
    public string? Source { get; set; }   // ezlm | msix | script | npm | path
    public string? State { get; set; }    // not_installed | installed | connected | needs_update | needs_64k | needs_tools | broken
    public string? Detail { get; set; }
    public string? Config { get; set; }
    public int? MinCtx { get; set; }
    public List<LaunchMode>? Launch { get; set; }
    public CheckResult? LastCheck { get; set; }

    /// The state to show; without one (an older engine), today's installed/connected logic.
    public string Shown => !Supported ? "unsupported" : !string.IsNullOrEmpty(State) ? State! : Connected ? "connected" : Installed ? "installed" : "not_installed";
    /// The logo key: app-<LogoKey>.png.
    public string LogoKey => string.IsNullOrEmpty(Icon) ? Id : Icon!;
    /// One button per launch mode; an older engine gets one "Open <Name>".
    public List<LaunchMode> Modes => Launch is { Count: > 0 } l ? l : new() { new LaunchMode { Id = "", Label = $"Open {Name}" } };
}

public class LaunchMode { public string Id { get; set; } = ""; public string Label { get; set; } = ""; }

/// The newest check or launch of an app. An OK Msg starts lowercase so it reads after "Checked 2 min ago: ".
public class CheckResult
{
    public DateTime At { get; set; }
    public bool Ok { get; set; }
    public bool ToolCall { get; set; }
    public double Secs { get; set; }
    public string? Stage { get; set; } // model | preflight | tui | launch
    public string? Msg { get; set; }
    public string? LogPath { get; set; }
}

/// What the Connect tiles and launch buttons say about an app, kept free of
/// UI types so the self-test can check it. Same words as the Mac app.
public static class AppText
{
    public enum Tone { Ok, Dim, Gold, Bad }

    public static (string Text, Tone Tone) Status(Target t)
    {
        var detail = !string.IsNullOrEmpty(t.Detail) ? t.Detail! : t.LastCheck?.Msg ?? "";
        return t.Shown switch
        {
            "unsupported" => (string.IsNullOrEmpty(t.Why) ? "Not available on this computer" : t.Why!, Tone.Dim),
            "connected" => ("✓ Connected", Tone.Ok),
            "installed" => ("Installed, not connected yet", Tone.Dim),
            "not_installed" => ("Not installed: EZLM will install it", Tone.Dim),
            "needs_update" => ("Points at an old address or key: EZLM fixes it when you open it", Tone.Gold),
            "needs_64k" => (detail != "" ? detail : $"{t.Name} needs a 64K context. Connect again to give the model the room (it may run slower), or pick a smaller model.", Tone.Gold),
            "needs_tools" => (detail != "" ? detail : $"This model can't use tools, so {t.Name} can't edit files with it. Pick a model marked Tools, or use Chat.", Tone.Gold),
            "broken" => ((t.LastCheck?.Stage == "launch" ? "Couldn't open last time" : "The last check failed") + (detail != "" ? ": " + detail : "."), Tone.Bad),
            _ => (detail != "" ? detail : t.Installed ? "Installed, not connected yet" : "Not installed: EZLM will install it", Tone.Dim),
        };
    }

    /// "2.2.5 · installed by EZLM" or "1.19.5 · yours"; "" without a version.
    public static string VersionLine(Target t) =>
        string.IsNullOrEmpty(t.Version) ? "" : $"{t.Version} · " + (t.Source == "ezlm" ? "installed by EZLM" : "yours");

    /// "Checked 2 min ago: answered and used a tool (14 s)" after a check that passed; "" otherwise.
    public static string CheckLine(Target t, DateTime now) =>
        t.LastCheck is { Ok: true } c && !string.IsNullOrEmpty(c.Msg) && c.At > DateTime.MinValue ? $"Checked {Ago(c.At, now)}: {c.Msg}" : "";

    /// "just now", "2 min ago", "3 h ago", "yesterday", "Nov 1": the words the
    /// chat list (ChatStore.Ago), the Mac app and the web page use.
    public static string Ago(DateTime at, DateTime now)
    {
        var s = (now.ToUniversalTime() - at.ToUniversalTime()).TotalSeconds;
        if (s < 60) return "just now";
        if (s < 3600) return $"{(int)(s / 60)} min ago";
        if (s < 86400) return $"{(int)(s / 3600)} h ago";
        if (s < 172800) return "yesterday";
        return at.ToLocalTime().ToString("MMM d", System.Globalization.CultureInfo.InvariantCulture);
    }

    /// "A", "A and B", "A, B and C".
    public static string And(IReadOnlyList<string> xs) =>
        xs.Count <= 2 ? string.Join(" and ", xs) : string.Join(", ", xs.Take(xs.Count - 1)) + " and " + xs[^1];

    /// The pixel wordmark drawn when an app's logo file is missing: (word, bright) parts.
    public static (string Word, bool Bright)[] Wordmark(string id) => id switch
    {
        "opencode" => new[] { ("open", false), ("code", true) },
        "hermes" => new[] { ("hermes", true) },
        "dsh" => new[] { ("dsh", true) },
        _ => Array.Empty<(string, bool)>(),
    };
}

public class Status
{
    public bool Deployed { get; set; }
    public bool Healthy { get; set; }
    public bool Starting { get; set; }
    public ServerConfig? Config { get; set; }
    public string? LocalUrl { get; set; }
    public string? LanUrl { get; set; }
    public string? Busy { get; set; }
    public TuneHint? Tune { get; set; }
    public bool UserStopped { get; set; }
    public string? StartError { get; set; }
    public bool Unmanaged { get; set; }
    public string Version { get; set; } = "";
    public CtxOption? Hermes64 { get; set; }
    public List<Target> Targets { get; set; } = new();
}

public class CtxOption { public bool Fits { get; set; } public double TokPerSec { get; set; } public double NowTokPerSec { get; set; } }

public class UpdateRelease { public string Version { get; set; } = ""; public string Name { get; set; } = ""; public string Notes { get; set; } = ""; public string Page { get; set; } = ""; public string Asset { get; set; } = ""; public ulong Size { get; set; } }
public class UpdateInfo { public string Current { get; set; } = ""; public bool Available { get; set; } public UpdateRelease? Latest { get; set; } public string? Error { get; set; } }
public class DriveInfo { public string Path { get; set; } = ""; public string Label { get; set; } = ""; public ulong Free { get; set; } public ulong Total { get; set; } public bool Current { get; set; } }
public class StorageInfo { public string Dir { get; set; } = ""; public bool Custom { get; set; } public ulong Free { get; set; } public ulong ModelsBytes { get; set; } public ulong Elsewhere { get; set; } public List<DriveInfo> Drives { get; set; } = new(); }

public class TuneHint { public string Why { get; set; } = ""; public string Quant { get; set; } = ""; }

public class EngineEvent
{
    public string Job { get; set; } = "";
    public string Step { get; set; } = "";
    public string State { get; set; } = "";
    public string? Msg { get; set; }
    public ulong? Done { get; set; }
    public ulong? Total { get; set; }
    public double? Bps { get; set; }
    public JsonElement? Data { get; set; }

    /// Test results ride in Data; other jobs put other shapes there.
    public TestResult? TestResult
    {
        get
        {
            if (Data is not { ValueKind: JsonValueKind.Object } d || !d.TryGetProperty("pass", out _)) return null;
            try { return d.Deserialize<TestResult>(Engine.Json); } catch { return null; }
        }
    }
}

public class TestResult { public string Name { get; set; } = ""; public bool Pass { get; set; } public string Detail { get; set; } = ""; public double Secs { get; set; } }

public class Live
{
    public ulong RamTotal { get; set; }
    public ulong RamAvail { get; set; }
    public ulong GpuTotal { get; set; }
    public ulong GpuFree { get; set; }
    public bool Unified { get; set; }
    public bool Healthy { get; set; }
    public bool Starting { get; set; }
    public double TokPerSec { get; set; }
    public int Requests { get; set; }
    public ulong Tokens { get; set; }
    public long UptimeS { get; set; }
    public int Queued { get; set; }
    public string? Phase { get; set; }
    public int PromptDone { get; set; }
    public int PromptTotal { get; set; }
    public int Written { get; set; }
    public ulong LoadDone { get; set; }
    public ulong LoadTotal { get; set; }
    public ulong GpuPlan { get; set; }
    public ulong RamPlan { get; set; }
}

public class LogLines { public List<string> Lines { get; set; } = new(); }

public class InstalledModel
{
    public string ModelId { get; set; } = "";
    public string Name { get; set; } = "";
    public string? Vendor { get; set; }
    public string? Country { get; set; }
    public string Quant { get; set; } = "";
    public ulong Bytes { get; set; }
    public bool Running { get; set; }
    public Plan? Plan { get; set; }
}

public class InstalledList { public List<InstalledModel> Models { get; set; } = new(); }

public static class Fmt
{
    const double GB = 1073741824;
    public static string Gb(ulong b) { var v = b / GB; return v < 1 ? $"{b / 1048576.0:0} MB" : v < 10 ? $"{v:0.0} GB" : $"{v:0} GB"; }
    public static string Uptime(long s) => s < 60 ? $"{s}s" : s < 3600 ? $"{s / 60}m" : s < 86400 ? $"{s / 3600}h {s % 3600 / 60}m" : $"{s / 86400}d {s % 86400 / 3600}h";
    public static string Masked(string k) => (k.Length > 9 ? k[..9] : k) + new string('•', 12);
}

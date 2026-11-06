using System.Diagnostics;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace EZLM;

/// Runs the bundled Go engine (`ezlm engine`) and talks to its local API.
/// The engine exits when this app closes its stdin, so it never outlives us.
public sealed class Engine : IDisposable
{
    Process? proc;
    readonly HttpClient http = new() { Timeout = TimeSpan.FromMinutes(5) };
    string baseUrl = "";
    bool disposing;

    /// The engine's process id (0 when it isn't running).
    public int Pid => proc is { HasExited: false } p ? p.Id : 0;

    /// Raised if the engine process dies while the app is open.
    public event Action? Exited;

    /// %LOCALAPPDATA%\EZLM: the app's own logs (engine.log, app.log).
    public static string DataDir
    {
        get
        {
            var d = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "EZLM");
            Directory.CreateDirectory(d);
            return d;
        }
    }

    public static readonly JsonSerializerOptions Json = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        PropertyNameCaseInsensitive = true,
        NumberHandling = JsonNumberHandling.AllowReadingFromString,
    };

    public static bool Demo => Environment.GetCommandLineArgs().Contains("--demo") || Environment.GetEnvironmentVariable("EZLM_DEMO") != null;

    static string BinaryPath()
    {
        var env = Environment.GetEnvironmentVariable("EZLM_ENGINE");
        if (!string.IsNullOrEmpty(env)) return env;
        return Path.Combine(AppContext.BaseDirectory, "Engine", "ezlm.exe");
    }

    public async Task StartAsync()
    {
        var bin = BinaryPath();
        if (!File.Exists(bin)) throw new Exception("EZLM's engine is missing from the app. Reinstall EZLM.");
        StopProcess();
        var psi = new ProcessStartInfo(bin, Demo ? "engine --demo" : "engine")
        {
            RedirectStandardOutput = true,
            RedirectStandardInput = true,
            RedirectStandardError = true,
            UseShellExecute = false,
            CreateNoWindow = true,
        };
        // Snapshots: the demo apps' starting states (see Snap.cs).
        var a = Environment.GetCommandLineArgs();
        int da = Array.IndexOf(a, "--demo-apps");
        if (Demo && da >= 0 && da + 1 < a.Length) psi.Environment["EZLM_DEMO_APPS"] = a[da + 1];
        var p = Process.Start(psi) ?? throw new Exception("Couldn't start EZLM's engine.");
        proc = p;
        p.EnableRaisingEvents = true;
        p.Exited += (_, _) => { if (!disposing && ReferenceEquals(proc, p)) Exited?.Invoke(); };
        _ = PumpStderr(p);
        var lineTask = p.StandardOutput.ReadLineAsync();
        if (await Task.WhenAny(lineTask, Task.Delay(TimeSpan.FromSeconds(30))) != lineTask)
            throw new Exception("EZLM's engine didn't start within 30 seconds. Your antivirus may be scanning it; try again in a moment.");
        var line = await lineTask ?? throw new Exception("EZLM's engine stopped while starting. Your antivirus may have blocked it.");
        var hello = JsonSerializer.Deserialize<Hello>(line, Json)!;
        baseUrl = hello.Url;
        http.DefaultRequestHeaders.Remove("X-EZLM-Token");
        http.DefaultRequestHeaders.Add("X-EZLM-Token", hello.Token);
    }

    /// Engine diagnostics go to engine.log (kept under ~2 MB) for support.
    static async Task PumpStderr(Process p)
    {
        try
        {
            var path = Path.Combine(DataDir, "engine.log");
            if (File.Exists(path) && new FileInfo(path).Length > 2_000_000) File.Delete(path);
            using var w = new StreamWriter(path, append: true) { AutoFlush = true };
            await w.WriteLineAsync($"--- {DateTime.Now:O} engine started (pid {p.Id})");
            string? l;
            while ((l = await p.StandardError.ReadLineAsync()) != null) await w.WriteLineAsync(l);
        }
        catch { }
    }

    void StopProcess()
    {
        var p = proc;
        proc = null;
        if (p == null) return;
        try { p.StandardInput.Close(); } catch { }
        try { if (!p.HasExited && !p.WaitForExit(1500)) p.Kill(); } catch { }
    }

    record Hello(string Url, string Token);

    public async Task<T> Get<T>(string path)
    {
        var r = await http.GetAsync($"{baseUrl}/api/{path}");
        return await Read<T>(r);
    }

    public async Task<T> Post<T>(string path, object? body = null)
    {
        var content = new StringContent(JsonSerializer.Serialize(body ?? new { }), Encoding.UTF8, "application/json");
        var r = await http.PostAsync($"{baseUrl}/api/{path}", content);
        return await Read<T>(r);
    }

    public Task Post(string path, object? body = null) => Post<JsonElement>(path, body);

    static async Task<T> Read<T>(HttpResponseMessage r)
    {
        var s = await r.Content.ReadAsStringAsync();
        if (!r.IsSuccessStatusCode)
        {
            string msg = $"Request failed ({(int)r.StatusCode})";
            try { msg = JsonSerializer.Deserialize<ErrorBody>(s, Json)?.Error ?? msg; } catch { }
            throw new Exception(msg);
        }
        return JsonSerializer.Deserialize<T>(s, Json)!;
    }

    record ErrorBody(string Error);

    /// Live job events (server-sent events), reconnecting if the stream drops.
    public async Task ListenAsync(Action<EngineEvent> onEvent, CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            try
            {
                var req = new HttpRequestMessage(HttpMethod.Get, $"{baseUrl}/api/events");
                using var resp = await http.SendAsync(req, HttpCompletionOption.ResponseHeadersRead, ct);
                using var stream = await resp.Content.ReadAsStreamAsync(ct);
                using var reader = new StreamReader(stream);
                while (!ct.IsCancellationRequested)
                {
                    var line = await reader.ReadLineAsync(ct);
                    if (line == null) break;
                    if (!line.StartsWith("data: ")) continue;
                    EngineEvent? e = null;
                    try { e = JsonSerializer.Deserialize<EngineEvent>(line[6..], Json); } catch { }
                    if (e != null) onEvent(e);
                }
            }
            catch when (!ct.IsCancellationRequested) { }
            await Task.Delay(500, ct).ContinueWith(_ => { });
        }
    }

    public void Dispose()
    {
        disposing = true;
        StopProcess();
    }
}

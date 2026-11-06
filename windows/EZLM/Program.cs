using Microsoft.UI.Xaml;

namespace EZLM;

public static partial class Program
{
    [System.Runtime.InteropServices.DllImport("user32.dll", CharSet = System.Runtime.InteropServices.CharSet.Unicode)]
    static extern IntPtr FindWindow(string? cls, string title);
    [System.Runtime.InteropServices.DllImport("user32.dll")]
    static extern bool ShowWindow(IntPtr h, int cmd);
    [System.Runtime.InteropServices.DllImport("user32.dll")]
    static extern bool SetForegroundWindow(IntPtr h);

    static Mutex? single;

    [STAThread]
    static void Main()
    {
        // Developer check: Chat's logic, in a throwaway folder; no window, no engine.
        var argv = Environment.GetCommandLineArgs();
        int st = Array.IndexOf(argv, "--selftest");
        if (st >= 0)
        {
            Environment.Exit(SelfTest.Run(st + 1 < argv.Length ? argv[st + 1] : Path.Combine(Path.GetTempPath(), "ezlm-selftest.txt")));
            return;
        }

        // At login (HKCU Run entry): start the model server in the background
        // and exit without showing a window.
        if (Environment.GetCommandLineArgs().Contains("--autostart"))
        {
            try
            {
                var engine = Path.Combine(AppContext.BaseDirectory, "Engine", "ezlm.exe");
                var p = System.Diagnostics.Process.Start(new System.Diagnostics.ProcessStartInfo(engine, "start --login") { UseShellExecute = false, CreateNoWindow = true });
                p?.WaitForExit(10 * 60 * 1000); // prewarming a big model can take a few minutes
            }
            catch { }
            return;
        }

        // One EZLM at a time: a second launch brings the open window forward
        // instead of starting a second engine that could fight over the server.
        single = new Mutex(true, Engine.Demo ? @"Local\EZLM-demo" : @"Local\EZLM-app", out var first);
        if (!first)
        {
            var h = FindWindow(null, "EZLM");
            if (h != IntPtr.Zero) { ShowWindow(h, 9 /* SW_RESTORE */); SetForegroundWindow(h); }
            return;
        }
        WinRT.ComWrappersSupport.InitializeComWrappers();
        Application.Start(p =>
        {
            var ctx = new Microsoft.UI.Dispatching.DispatcherQueueSynchronizationContext(Microsoft.UI.Dispatching.DispatcherQueue.GetForCurrentThread());
            SynchronizationContext.SetSynchronizationContext(ctx);
            _ = new App();
        });
    }
}


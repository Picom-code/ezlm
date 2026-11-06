using Microsoft.UI.Xaml;

namespace EZLM;

public partial class App : Application
{
    Window? window;

    public App()
    {
        InitializeComponent();
        UnhandledException += (_, e) => { Shell.Log(e.Exception); e.Handled = true; };
        // Off the window's thread: a background task's failure is written down (and
        // otherwise ignored, as before), and a crash leaves its reason in app.log,
        // which Report a problem sends along.
        System.Threading.Tasks.TaskScheduler.UnobservedTaskException += (_, e) => { Shell.Log(e.Exception); e.SetObserved(); };
        AppDomain.CurrentDomain.UnhandledException += (_, e) => { if (e.ExceptionObject is Exception ex) Shell.Log(ex); };
    }

    protected override void OnLaunched(LaunchActivatedEventArgs args)
    {
        var shell = new Shell();
        window = shell;
        if (Shell.SnapDir != null) shell.StartHidden(); // developer check: nothing shows on screen
        else window.Activate();
    }
}

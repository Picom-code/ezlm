using Microsoft.UI;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using static EZLM.Ui;

namespace EZLM;

// Settings: updates (check, download, install) and where models are stored.
public sealed partial class Shell
{
    UpdateInfo? updateInfo;
    bool updateBusy;
    string updateNote = "";
    BlockBar? updateBar;
    TextBlock? updateStat;

    StorageInfo? storage;
    string? pendingDir;     // the drive or folder picked, not saved yet
    bool moveModels = true;
    bool storageBusy;
    BlockBar? storageBar;
    TextBlock? storageStat;

    /// Checks GitHub for a newer EZLM (the engine caches it for an hour).
    async Task CheckUpdate(bool force = false)
    {
        try { updateInfo = await engine.Get<UpdateInfo>(force ? "update?force=1" : "update"); }
        catch (Exception ex) { updateInfo = new UpdateInfo { Error = ex.Message }; }
    }

    async Task RefreshStorage()
    {
        try { storage = await engine.Get<StorageInfo>("storage"); } catch (Exception ex) { Log(ex); }
    }

    void SettingsPane(StackPanel b)
    {
        UpdatesAndStorage(b);
        b.Children.Add(Section("About"));
        b.Children.Add(Txt(AboutApps, 11, Dim).Also(t => t.IsTextSelectionEnabled = true));
    }

    /// The app logos' attribution (the same line as Assets/Logos/NOTICE.md and the Mac app).
    public const string AboutApps = "App logos identify the apps EZLM connects to and are trademarks of their owners: OpenCode (Anomaly Innovations, Inc.; square logo from github.com/anomalyco/opencode, MIT), Hermes Agent (Nous Research, Inc.; app icon from github.com/NousResearch/hermes-agent, MIT), DeepSeek Harness (DeepSeek; whale icon from github.com/deepseek-ai/deepseek-harness, MIT). EZLM is compatible with these apps but is not affiliated with or endorsed by their makers.";

    void UpdatesAndStorage(StackPanel b)
    {
        Header(b, "Settings", "Keep EZLM up to date and choose where models are saved.");

        // Updates
        b.Children.Add(Section("Updates"));
        var cur = status?.Version is { Length: > 0 } v ? v : updateInfo?.Current ?? "";
        var upd = Col(10);
        if (updateInfo is { Available: true, Latest: { } latest })
        {
            upd.Children.Add(Runs(14, ("EZLM " + latest.Version, Ok), (" is available. You have " + cur + ".", Dim)));
            var notes = (latest.Notes ?? "").Split('\n').Select(l => l.Trim()).Where(l => l.Length > 0 && !l.StartsWith("#") && !l.StartsWith("-")).Take(2);
            foreach (var n in notes) upd.Children.Add(Txt(n.Length > 160 ? n[..160] + "…" : n, 12, Dim));
            var go = Btn(updateBusy ? "Updating…" : $"Update to {latest.Version}  ↵", Kind.Primary, () => _ = StartUpdate());
            go.IsEnabled = !updateBusy && !installRunning;
            upd.Children.Add(Row(12, go, Txt($"{Fmt.Gb(latest.Size)} download. Your model keeps running; EZLM restarts itself.", 11, Dim).Also(t => t.VerticalAlignment = VerticalAlignment.Center)));
        }
        else
        {
            var line = updateInfo == null ? $"EZLM {cur}. Checking for updates…"
                : !string.IsNullOrEmpty(updateInfo.Error) ? $"EZLM {cur}. {updateInfo.Error}."
                : $"EZLM {cur} is the latest version.";
            upd.Children.Add(Txt(line, 13, Ink));
            upd.Children.Add(Btn("Check for updates", Kind.Normal, async () => { updateInfo = null; ShowSection(); await CheckUpdate(true); if (section == "Settings") ShowSection(); }).Also(x => x.IsEnabled = !updateBusy));
        }
        updateBar = new BlockBar(6) { Visibility = updateBusy ? Visibility.Visible : Visibility.Collapsed };
        updateStat = Txt(updateNote, 11, Dim);
        upd.Children.Add(updateBar); upd.Children.Add(updateStat);
        b.Children.Add(upd);

        // Model storage
        b.Children.Add(Section("Where models are saved"));
        if (storage == null)
        {
            b.Children.Add(Row(10, new Spinner(), Txt("Looking at your drives", 12, Dim)));
            _ = RefreshStorage().ContinueWith(_ => Ui(() => { if (section == "Settings") ShowSection(); }));
            return;
        }
        var st = storage;
        b.Children.Add(Runs(13, ("New downloads go to ", Dim), (st.Dir, Ink), ($"  ({Fmt.Gb(st.Free)} free)", Dim)).Also(t => t.Margin = new Thickness(0, 0, 0, 4)));
        if (st.ModelsBytes > 0) b.Children.Add(Txt($"EZLM's models on this computer take {Fmt.Gb(st.ModelsBytes)}" + (st.Elsewhere > 0 ? $", {Fmt.Gb(st.Elsewhere)} of it in other folders." : "."), 12, Dim));

        var grid = new Grid { ColumnSpacing = 10, RowSpacing = 10, Margin = new Thickness(0, 14, 0, 0) };
        grid.ColumnDefinitions.Add(new ColumnDefinition());
        grid.ColumnDefinitions.Add(new ColumnDefinition());
        int idx = 0;
        foreach (var d in st.Drives)
        {
            if (idx % 2 == 0) grid.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
            bool picked = pendingDir != null ? SamePath(pendingDir, d.Path) : d.Current;
            var bar = new BlockBar(6); bar.SetFill(d.Total > 0 ? 1 - (double)d.Free / d.Total : 0, d.Free < (20UL << 30) ? Gold : Signal);
            var name = string.IsNullOrEmpty(d.Label) ? d.Path : $"{d.Label} ({d.Path.TrimEnd('\\')})";
            var card = new Border
            {
                Padding = new Thickness(12), BorderThickness = new Thickness(1),
                BorderBrush = B(picked ? Signal : Faint), Background = B(picked ? C(0xb9a8ff, 18) : Colors.Transparent),
                Child = Col(6, Row(8, Txt(name, 13, Ink, true), d.Current ? Txt("in use", 10, Ok) : new Border()),
                    Txt($"{Fmt.Gb(d.Free)} free of {Fmt.Gb(d.Total)}", 11, Dim), bar),
            };
            Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(card, name);
            var path = d.Path;
            card.Tapped += (_, _) => { if (storageBusy) return; pendingDir = d.Current ? null : path; ShowSection(); };
            Grid.SetRow(card, idx / 2); Grid.SetColumn(card, idx % 2);
            grid.Children.Add(card);
            idx++;
        }
        b.Children.Add(grid);

        var folderBtn = Btn("Choose a folder…", Kind.Normal, () => _ = PickFolder());
        folderBtn.IsEnabled = !storageBusy;
        b.Children.Add(folderBtn.Also(x => x.Margin = new Thickness(0, 12, 0, 0)));

        if (pendingDir != null)
        {
            var target = StorageTarget(pendingDir, st);
            var panel = Col(8).Also(p => p.Margin = new Thickness(0, 14, 0, 0));
            panel.Children.Add(Runs(13, ("Models will go to ", Dim), (target, Ink)));
            if (st.ModelsBytes > 0)
            {
                var mv = new CheckBox { Content = Txt($"Also move the models already downloaded ({Fmt.Gb(st.ModelsBytes)})"), IsChecked = moveModels, IsEnabled = !storageBusy };
                mv.Checked += (_, _) => moveModels = true; mv.Unchecked += (_, _) => moveModels = false;
                panel.Children.Add(mv);
                panel.Children.Add(Txt(moveModels ? "Your running model pauses for a moment while its file moves." : "Models stay where they are and keep working; only new downloads go to the new place.", 11, Dim));
            }
            var save = Btn(storageBusy ? "Saving…" : "Save  ↵", Kind.Primary, () => _ = SaveStorage(target));
            save.IsEnabled = !storageBusy && !installRunning;
            panel.Children.Add(Row(10, save, Btn("Cancel", Kind.Normal, () => { pendingDir = null; ShowSection(); }).Also(x => x.IsEnabled = !storageBusy)));
            b.Children.Add(panel);
        }
        storageBar = new BlockBar(6) { Visibility = storageBusy ? Visibility.Visible : Visibility.Collapsed, Margin = new Thickness(0, 10, 0, 0) };
        storageStat = Txt("", 11, Dim);
        b.Children.Add(storageBar); b.Children.Add(storageStat);
    }

    static bool SamePath(string a, string b) => string.Equals(a.TrimEnd('\\'), b.TrimEnd('\\'), StringComparison.OrdinalIgnoreCase);

    /// Mirrors the engine: models live in an "EZLM Models" folder inside what's picked
    /// (so removing EZLM only ever deletes its own files). Picking the drive already
    /// in use keeps the current folder.
    static string StorageTarget(string picked, StorageInfo st)
    {
        if (st.Drives.FirstOrDefault(d => d.Current) is { } cur && SamePath(cur.Path, picked)) return st.Dir;
        var name = System.IO.Path.GetFileName(picked.TrimEnd('\\'));
        return name.Contains("ezlm", StringComparison.OrdinalIgnoreCase) ? picked.TrimEnd('\\') : System.IO.Path.Combine(picked, "EZLM Models");
    }

    /// First-run version of the storage choice: before anything is downloaded.
    async Task StorageDialog()
    {
        if (dialogOpen) return;
        await RefreshStorage();
        if (storage == null) return;
        string? choice = null;
        var list = Col(8);
        var preview = Txt("", 12, Dim);
        void Paint()
        {
            list.Children.Clear();
            foreach (var d in storage!.Drives)
            {
                bool on = choice != null ? SamePath(choice, d.Path) : d.Current;
                var name = string.IsNullOrEmpty(d.Label) ? d.Path : $"{d.Label} ({d.Path.TrimEnd('\\')})";
                var row = new Border
                {
                    Padding = new Thickness(10, 8, 10, 8), BorderThickness = new Thickness(1), BorderBrush = B(on ? Signal : Faint), Background = B(on ? C(0xb9a8ff, 18) : Colors.Transparent),
                    Child = Row(10, Txt(name, 13, Ink, true), Txt($"{Fmt.Gb(d.Free)} free", 12, Dim)),
                };
                var path = d.Path;
                row.Tapped += (_, _) => { choice = d.Current ? null : path; Paint(); };
                list.Children.Add(row);
            }
            preview.Text = "Models will go to " + (choice == null ? storage.Dir : StorageTarget(choice, storage));
        }
        Paint();
        var pick = Btn("Choose a folder…", Kind.Normal, async () =>
        {
            try
            {
                var picker = new Windows.Storage.Pickers.FolderPicker();
                WinRT.Interop.InitializeWithWindow.Initialize(picker, WinRT.Interop.WindowNative.GetWindowHandle(this));
                picker.FileTypeFilter.Add("*");
                if (await picker.PickSingleFolderAsync() is { } f) { choice = f.Path; Paint(); }
            }
            catch (Exception ex) { Log(ex); }
        });
        var d = new ContentDialog
        {
            Title = "Where should models be saved?",
            Content = Col(10, Txt("Models are big (often 2 to 70 GB). Pick the drive with the most room.", 12, Dim), list, pick, preview),
            PrimaryButtonText = "Save", CloseButtonText = "Cancel", DefaultButton = ContentDialogButton.Primary,
            XamlRoot = root.XamlRoot, RequestedTheme = ElementTheme.Dark,
        };
        if (await ShowDialog(d) != ContentDialogResult.Primary || choice == null) return;
        storageBusy = true;
        try { await engine.Post("storage", new { dir = StorageTarget(choice, storage), move = false }); }
        catch (Exception ex) { storageBusy = false; Toast(ex.Message, Bad); }
    }

    async Task PickFolder()
    {
        try
        {
            var picker = new Windows.Storage.Pickers.FolderPicker();
            WinRT.Interop.InitializeWithWindow.Initialize(picker, WinRT.Interop.WindowNative.GetWindowHandle(this));
            picker.FileTypeFilter.Add("*");
            var folder = await picker.PickSingleFolderAsync();
            if (folder == null) return;
            pendingDir = folder.Path;
            ShowSection();
        }
        catch (Exception ex) { Log(ex); Toast("Couldn't open the folder picker: " + ex.Message, Bad); }
    }

    async Task SaveStorage(string target)
    {
        if (storageBusy || storage == null) return;
        storageBusy = true;
        ShowSection();
        try { await engine.Post("storage", new { dir = target, move = moveModels && storage.ModelsBytes > 0 }); }
        catch (Exception ex) { storageBusy = false; Toast(ex.Message, Bad); ShowSection(); }
    }

    void OnStorageEvent(EngineEvent e)
    {
        if (!storageBusy) return; // not a change this window started
        if (e.Step == "done")
        {
            storageBusy = false;
            if (e.State == "fail") Taskbar.Failed(Hwnd); else Taskbar.Clear(Hwnd);
            if (e.State != "cancelled") Taskbar.Flash(Hwnd);
            if (e.State == "ok") { pendingDir = null; scan = null; chooseBuiltFor = null; }
            else Toast(e.Msg ?? "Couldn't change where models are saved.", Bad);
            _ = RefreshStorage().ContinueWith(_ => Ui(() => { if (section == "Settings") ShowSection(); else if (screen == Screen.Choose) _ = RunScan(); }));
            return;
        }
        if (e.State == "progress" && e.Total is > 0 && e.Done is { } d)
        {
            storageBar?.SetFill((double)d / e.Total.Value, Signal);
            Taskbar.Progress(Hwnd, (double)d / e.Total.Value);
            if (storageBar != null) storageBar.Visibility = Visibility.Visible;
            if (storageStat != null) storageStat.Text = $"{Fmt.Gb(d)} of {Fmt.Gb(e.Total.Value)} moved";
        }
        else if (e.State == "ok" && !string.IsNullOrEmpty(e.Msg)) Toast(e.Msg!, Ok);
        else if (!string.IsNullOrEmpty(e.Msg) && storageStat != null) storageStat.Text = e.Msg!;
    }

    // MARK: updating

    async Task StartUpdate()
    {
        if (updateBusy) return;
        updateBusy = true; updateNote = "Starting the download…";
        if (section == "Settings") ShowSection();
        try { await engine.Post("update/download"); }
        catch (Exception ex) { updateBusy = false; updateNote = ex.Message; if (section == "Settings" || section == "Overview") ShowSection(); }
    }

    /// Other copies of EZLM's engine (the login start pre-reading a big model,
    /// or a command run by hand) keep Engine\ezlm.exe open, and the installer
    /// can't replace a file that's in use. Only processes running exactly this
    /// install's engine are stopped, never anything matched by name alone; the
    /// window's own engine exits with the window.
    void StopEngineHelpers()
    {
        var exe = System.IO.Path.Combine(AppContext.BaseDirectory, "Engine", "ezlm.exe");
        foreach (var p in System.Diagnostics.Process.GetProcessesByName("ezlm"))
        {
            try
            {
                if (p.Id != engine.Pid && string.Equals(p.MainModule?.FileName, exe, StringComparison.OrdinalIgnoreCase))
                {
                    p.Kill();
                    p.WaitForExit(3000);
                }
            }
            catch (Exception ex) { Log(ex); }
        }
    }

    void OnUpdateEvent(EngineEvent e)
    {
        if (!updateBusy) return; // not an update this window started
        if (e.Step == "download" && e.State == "progress" && e.Total is > 0 && e.Done is { } d)
        {
            updateNote = $"Downloading: {Fmt.Gb(d)} of {Fmt.Gb(e.Total.Value)}";
            if (updateBar != null) { updateBar.Visibility = Visibility.Visible; updateBar.SetFill((double)d / e.Total.Value, Signal); }
            if (updateStat != null) updateStat.Text = updateNote;
            return;
        }
        if (e.Step == "download" && e.State == "ok" && !string.IsNullOrEmpty(e.Msg))
        {
            // The installer replaces the app's files, so EZLM steps aside and the
            // installer opens it again when it's done. The model server keeps running.
            try
            {
                StopEngineHelpers();
                System.Diagnostics.Process.Start(new System.Diagnostics.ProcessStartInfo(e.Msg!, "/SILENT /SUPPRESSMSGBOXES /NORESTART /SP- /CLOSEAPPLICATIONS /UPDATE=1") { UseShellExecute = true });
                updateNote = "Installing the update. EZLM will open again in a moment.";
                if (updateStat != null) updateStat.Text = updateNote;
                Toast("Installing the update. EZLM will open again in a moment.", Ok);
                _ = Task.Delay(1200).ContinueWith(_ => Ui(() => Close()));
            }
            catch (Exception ex)
            {
                updateBusy = false;
                updateNote = "Couldn't start the installer: " + ex.Message;
                if (section == "Settings") ShowSection();
            }
            return;
        }
        if (e.Step == "done" && e.State != "ok")
        {
            updateBusy = false;
            updateNote = e.State == "cancelled" ? "Update cancelled." : (e.Msg ?? "The update didn't download.");
            if (section == "Settings" || section == "Overview") ShowSection();
        }
    }
}

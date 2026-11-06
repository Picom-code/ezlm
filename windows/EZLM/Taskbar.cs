using System.Runtime.InteropServices;

namespace EZLM;

/// The taskbar button shows how far a long job is (like a browser download)
/// and flashes when it ends while EZLM is in the background, so nobody has to
/// keep checking the window.
static class Taskbar
{
    [ComImport, Guid("ea1afb91-9e28-4b86-90e9-9e9f8a5eefaf"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
    interface ITaskbarList3
    {
        // ITaskbarList
        void HrInit();
        void AddTab(IntPtr hwnd);
        void DeleteTab(IntPtr hwnd);
        void ActivateTab(IntPtr hwnd);
        void SetActiveAlt(IntPtr hwnd);
        // ITaskbarList2
        void MarkFullscreenWindow(IntPtr hwnd, [MarshalAs(UnmanagedType.Bool)] bool fullscreen);
        // ITaskbarList3 (only the first two methods are used)
        void SetProgressValue(IntPtr hwnd, ulong completed, ulong total);
        void SetProgressState(IntPtr hwnd, int flags);
    }

    [ComImport, Guid("56fdf344-fd6d-11d0-958a-006097c9a090"), ClassInterface(ClassInterfaceType.None)]
    class TaskbarList { }

    const int NoProgress = 0, Normal = 0x2, Error = 0x4;

    static ITaskbarList3? list;
    static bool broken;

    static ITaskbarList3? List()
    {
        if (list != null || broken) return list;
        try { var l = (ITaskbarList3)new TaskbarList(); l.HrInit(); list = l; }
        catch (Exception ex) { broken = true; Shell.Log(ex); } // no taskbar (e.g. some kiosk shells): just skip it
        return list;
    }

    public static void Progress(IntPtr hwnd, double frac)
    {
        try { var l = List(); l?.SetProgressState(hwnd, Normal); l?.SetProgressValue(hwnd, (ulong)(Math.Clamp(frac, 0, 1) * 1000), 1000); }
        catch (Exception ex) { Shell.Log(ex); }
    }

    /// A red bar: the job stopped with an error.
    public static void Failed(IntPtr hwnd)
    {
        try { List()?.SetProgressState(hwnd, Error); } catch (Exception ex) { Shell.Log(ex); }
    }

    public static void Clear(IntPtr hwnd)
    {
        try { List()?.SetProgressState(hwnd, NoProgress); } catch (Exception ex) { Shell.Log(ex); }
    }

    [StructLayout(LayoutKind.Sequential)]
    struct FlashInfo { public uint Size; public IntPtr Hwnd; public uint Flags, Count, Timeout; }

    [DllImport("user32.dll")] static extern bool FlashWindowEx(ref FlashInfo info);
    [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll")] static extern bool IsIconic(IntPtr hwnd);

    /// Flashes the taskbar button until the window is opened again; nothing
    /// happens if it's in front (a minimized window can still count as the
    /// foreground one, so that's checked too).
    public static void Flash(IntPtr hwnd)
    {
        if (GetForegroundWindow() == hwnd && !IsIconic(hwnd)) return;
        var info = new FlashInfo { Size = (uint)Marshal.SizeOf<FlashInfo>(), Hwnd = hwnd, Flags = 0x2 | 0xC /* FLASHW_TRAY | FLASHW_TIMERNOFG */ };
        FlashWindowEx(ref info);
    }
}

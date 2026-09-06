param(
    [Parameter(Mandatory=$true)][int]$DesktopPID,
    [ValidateSet('status','close','open','pause','resume','quit','menu')][string]$Action = 'status'
)
$ErrorActionPreference = 'Stop'
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
using System.Text;
public static class SpeedLimitFreeTrayProbe {
    private delegate bool EnumProc(IntPtr h, IntPtr p);
    [DllImport("user32.dll")] private static extern bool EnumWindows(EnumProc cb, IntPtr p);
    [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr h, out uint p);
    [DllImport("user32.dll", CharSet=CharSet.Unicode)] private static extern int GetClassName(IntPtr h, StringBuilder s, int n);
    [DllImport("user32.dll", CharSet=CharSet.Unicode)] private static extern int GetWindowText(IntPtr h, StringBuilder s, int n);
    [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
    [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint msg, IntPtr w, IntPtr l);
    [StructLayout(LayoutKind.Sequential)] public struct IconIdentifier { public uint size; public IntPtr hwnd; public uint id; public Guid guid; }
    [StructLayout(LayoutKind.Sequential)] public struct Rect { public int left,top,right,bottom; }
    [DllImport("shell32.dll")] private static extern int Shell_NotifyIconGetRect(ref IconIdentifier identifier, out Rect rect);
    public static IntPtr Find(uint pid, bool tray) {
        IntPtr found=IntPtr.Zero;
        EnumWindows((h,p)=>{
            uint owner; GetWindowThreadProcessId(h,out owner); if(owner!=pid) return true;
            var cls=new StringBuilder(256);GetClassName(h,cls,256);
            var title=new StringBuilder(256);GetWindowText(h,title,256);
            if(tray ? cls.ToString()=="SpeedLimitFree.Tray" : title.ToString()=="SpeedLimitFree" && cls.ToString()!="SpeedLimitFree.Tray") {found=h;return false;}
            return true;
        },IntPtr.Zero);
        return found;
    }
    public static bool HasIcon(IntPtr hwnd) {
        var identifier=new IconIdentifier {size=(uint)Marshal.SizeOf(typeof(IconIdentifier)),hwnd=hwnd,id=1};Rect rect;
        return Shell_NotifyIconGetRect(ref identifier,out rect)==0;
    }
}
'@
$taskTray = [SpeedLimitFreeTrayProbe]::Find($DesktopPID, $true)
$taskWindow = [SpeedLimitFreeTrayProbe]::Find($DesktopPID, $false)
if ($Action -eq 'status') {
    @{ trayWindow = $taskTray -ne [IntPtr]::Zero; trayIcon = $taskTray -ne [IntPtr]::Zero -and [SpeedLimitFreeTrayProbe]::HasIcon($taskTray); windowFound = $taskWindow -ne [IntPtr]::Zero; windowVisible = $taskWindow -ne [IntPtr]::Zero -and [SpeedLimitFreeTrayProbe]::IsWindowVisible($taskWindow) } | ConvertTo-Json -Compress
} elseif ($Action -eq 'close') {
    if ($taskWindow -eq [IntPtr]::Zero) { throw 'Desktop window not found.' }
    if (-not [SpeedLimitFreeTrayProbe]::PostMessage($taskWindow,0x10,[IntPtr]::Zero,[IntPtr]::Zero)) { throw 'Could not send window close.' }
} else {
    if ($taskTray -eq [IntPtr]::Zero) { throw 'Tray window not found.' }
    if ($Action -eq 'menu') {
        [SpeedLimitFreeTrayProbe]::PostMessage($taskTray,0x8001,[IntPtr]::Zero,[IntPtr]0x7b) | Out-Null
    } else {
        $taskCommand = @{ open=1001; pause=1002; quit=1003; resume=1004 }[$Action]
        if (-not [SpeedLimitFreeTrayProbe]::PostMessage($taskTray,0x111,[IntPtr]$taskCommand,[IntPtr]::Zero)) { throw 'Could not send tray command.' }
    }
}

using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;

namespace SpeedLimitFreeInstaller {
    public static class Native {
        private delegate bool EnumProc(IntPtr h, IntPtr p);
        [DllImport("user32.dll")] private static extern bool EnumWindows(EnumProc cb, IntPtr p);
        [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr h, out uint p);
        [DllImport("user32.dll")] private static extern IntPtr GetShellWindow();
        [DllImport("user32.dll", CharSet=CharSet.Unicode)] private static extern int GetClassName(IntPtr h, StringBuilder s, int n);
        [DllImport("user32.dll")] private static extern bool PostMessage(IntPtr h, uint msg, IntPtr w, IntPtr l);
        [DllImport("kernel32.dll", CharSet=CharSet.Unicode, SetLastError=true)] private static extern IntPtr LoadLibraryEx(string name, IntPtr file, uint flags);
        [DllImport("kernel32.dll", CharSet=CharSet.Ansi, ExactSpelling=true)] private static extern IntPtr GetProcAddress(IntPtr library, string name);
        [DllImport("kernel32.dll")] private static extern bool FreeLibrary(IntPtr library);
        [UnmanagedFunctionPointer(CallingConvention.Winapi, CharSet=CharSet.Ansi, SetLastError=true)]
        private delegate IntPtr Open(string filter, int layer, short priority, ulong flags);
        [UnmanagedFunctionPointer(CallingConvention.Winapi, SetLastError=true)]
        private delegate bool Shutdown(IntPtr h, int how);
        [UnmanagedFunctionPointer(CallingConvention.Winapi, SetLastError=true)]
        private delegate bool Receive(IntPtr h, [Out] byte[] packet, uint length, out uint received, [Out] byte[] address);
        [UnmanagedFunctionPointer(CallingConvention.Winapi, SetLastError=true)]
        private delegate bool Close(IntPtr h);
        public static uint ShellProcess() {
            uint pid; GetWindowThreadProcessId(GetShellWindow(), out pid); return pid;
        }
        public static bool QuitDesktop(uint pid) {
            bool sent = false;
            EnumWindows((h,p) => {
                uint owner; GetWindowThreadProcessId(h, out owner);
                if (owner != pid) return true;
                var name = new StringBuilder(256); GetClassName(h, name, 256);
                if (name.ToString() == "SpeedLimitFree.Tray") {
                    sent = PostMessage(h, 0x111, new IntPtr(1003), IntPtr.Zero);
                    return false;
                }
                return true;
            }, IntPtr.Zero);
            return sent;
        }
        private static T Export<T>(IntPtr library, string name) where T: class {
            IntPtr address = GetProcAddress(library, name);
            if (address == IntPtr.Zero) throw new InvalidOperationException("Missing WinDivert export: " + name);
            return Marshal.GetDelegateForFunctionPointer(address, typeof(T)) as T;
        }
        public static uint[] DriverClients(string dllPath) {
            IntPtr library = LoadLibraryEx(dllPath, IntPtr.Zero, 0x100 | 0x800);
            if (library == IntPtr.Zero) throw new Win32Exception(Marshal.GetLastWin32Error());
            try {
                var open = Export<Open>(library, "WinDivertOpen");
                var close = Export<Close>(library, "WinDivertClose");
                var shutdown = Export<Shutdown>(library, "WinDivertShutdown");
                var recv = Export<Receive>(library, "WinDivertRecv");
                // REFLECT, SNIFF | RECV_ONLY | NO_INSTALL. Observe handles only.
                IntPtr handle = open("event == OPEN", 4, 0, 1 | 4 | 16);
                if (handle == new IntPtr(-1)) {
                    int error = Marshal.GetLastWin32Error();
                    if (error == 1060) return new uint[0];
                    throw new Win32Exception(error);
                }
                try {
                    // Drain only the existing handle snapshot, without blocking on future events.
                    if (!shutdown(handle, 1)) throw new Win32Exception(Marshal.GetLastWin32Error());
                    var clients = new HashSet<uint>();
                    var address = new byte[80];
                    var packet = new byte[65535];
                    for (int i = 0; i < 65536; i++) {
                        uint count;
                        if (!recv(handle, packet, (uint)packet.Length, out count, address)) {
                            int error = Marshal.GetLastWin32Error();
                            if (error == 232) { var result = new uint[clients.Count]; clients.CopyTo(result); return result; }
                            throw new Win32Exception(error);
                        }
                        // WINDIVERT_ADDRESS union at 16, REFLECT.ProcessId at union+8.
                        uint pid = BitConverter.ToUInt32(address, 24);
                        if (pid != Process.GetCurrentProcess().Id) clients.Add(pid);
                    }
                    throw new InvalidOperationException("Too many WinDivert handles to verify safe removal.");
                } finally { close(handle); }
            } finally { FreeLibrary(library); }
        }
    }
}

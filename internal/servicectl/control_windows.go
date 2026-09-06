// Package servicectl queries SCM without elevation and waits for elevated service actions.
package servicectl

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"speedlimitfree/internal/contracts"
)

func Status() (contracts.ServiceStatus, error) {
	data := filepath.Join(os.Getenv("ProgramData"), "SpeedLimitFree")
	r := contracts.ServiceStatus{State: "not installed", LogPath: data, ServiceLog: tail(filepath.Join(data, "service.log")), SetupLog: tail(filepath.Join(data, "setup.log"))}
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return r, fmt.Errorf("read Windows services: %w", err)
	}
	defer windows.CloseServiceHandle(m)
	name, _ := windows.UTF16PtrFromString(contracts.ServiceName)
	h, err := windows.OpenService(m, name, windows.SERVICE_QUERY_STATUS|windows.SERVICE_QUERY_CONFIG)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return r, nil
	}
	if err != nil {
		return r, fmt.Errorf("read service status: %w", err)
	}
	s := mgr.Service{Name: contracts.ServiceName, Handle: h}
	defer s.Close()
	r.Installed = true
	state, err := s.Query()
	if err != nil {
		return r, err
	}
	r.State = stateName(state.State)
	r.PID, r.ExitCode, r.ServiceExitCode = state.ProcessId, state.Win32ExitCode, state.ServiceSpecificExitCode
	config, err := s.Config()
	if err != nil {
		return r, err
	}
	r.Binary = config.BinaryPathName
	r.Startup = map[uint32]string{mgr.StartAutomatic: "Automatic", mgr.StartManual: "Manual", mgr.StartDisabled: "Disabled"}[config.StartType]
	return r, nil
}

func stateName(s svc.State) string {
	if name, ok := map[svc.State]string{svc.Stopped: "stopped", svc.StartPending: "starting", svc.StopPending: "stopping", svc.Running: "running", svc.Paused: "paused", svc.PausePending: "pausing", svc.ContinuePending: "resuming"}[s]; ok {
		return name
	}
	return "unknown"
}

func tail(path string) string {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		return "Cannot read log: " + err.Error()
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "Cannot read log: " + err.Error()
	}
	if info.Size() > 8192 {
		f.Seek(-8192, io.SeekEnd)
	}
	b, err := io.ReadAll(io.LimitReader(f, 8192))
	if err != nil {
		return "Cannot read log: " + err.Error()
	}
	return strings.TrimSpace(string(bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})))
}

// SHELLEXECUTEINFOW; use a process handle so launch is never mistaken for success.
type shellExecuteInfo struct {
	Size, Mask                        uint32
	Window                            uintptr
	Verb, File, Parameters, Directory *uint16
	Show                              int32
	Instance, IDList                  uintptr
	Class                             *uint16
	ClassKey                          uintptr
	HotKey                            uint32
	Icon                              uintptr
	Process                           windows.Handle
}

func Manage(ctx context.Context, action, dir string) (contracts.ServiceResult, error) {
	if action != "install" && action != "start" && action != "stop" && action != "restart" {
		return contracts.ServiceResult{}, errors.New("unknown service action")
	}
	script := filepath.Join(dir, "manage-service.ps1")
	for _, file := range []string{script, filepath.Join(dir, "service-common.ps1")} {
		if _, err := os.Stat(file); err != nil {
			return contracts.ServiceResult{}, fmt.Errorf("service manager is missing beside this app; extract the complete release folder: %w", err)
		}
	}
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return contracts.ServiceResult{}, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return contracts.ServiceResult{}, err
	}
	id := hex.EncodeToString(random[:])
	args := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script, "-Action", action, "-OwnerSID", u.User.Sid.String(), "-OperationID", id}
	for i := range args {
		args[i] = windows.EscapeArg(args[i])
	}
	code, err := runElevated(ctx, strings.Join(args, " "), dir)
	if err != nil {
		return contracts.ServiceResult{}, err
	}
	resultPath := filepath.Join(os.Getenv("ProgramData"), "SpeedLimitFree", "setup-results", id+".json")
	b, err := os.ReadFile(resultPath)
	if err != nil {
		return contracts.ServiceResult{}, fmt.Errorf("service action exited with code %d without a result; check setup.log in Settings: %w", code, err)
	}
	return decodeResult(b, id, action, code)
}

func decodeResult(b []byte, id, action string, code uint32) (contracts.ServiceResult, error) {
	var r contracts.ServiceResult
	if err := json.Unmarshal(bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf}), &r); err != nil {
		return r, fmt.Errorf("invalid setup result: %w", err)
	}
	if r.OperationID != id || r.Action != action {
		return r, errors.New("setup result does not match this operation")
	}
	if code != 0 || !r.Success {
		if r.Message == "" {
			r.Message = fmt.Sprintf("service action failed with exit code %d", code)
		}
		return r, errors.New(r.Message)
	}
	return r, nil
}

func runElevated(ctx context.Context, args, dir string) (uint32, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ole := windows.NewLazySystemDLL("ole32.dll")
	initialized, _, _ := ole.NewProc("CoInitializeEx").Call(0, 2)
	if int32(initialized) >= 0 {
		defer ole.NewProc("CoUninitialize").Call()
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	program, _ := windows.UTF16PtrFromString(filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"))
	parameters, _ := windows.UTF16PtrFromString(args)
	directory, _ := windows.UTF16PtrFromString(dir)
	info := shellExecuteInfo{Size: uint32(unsafe.Sizeof(shellExecuteInfo{})), Mask: 0x40 | 0x100 | 0x400, Verb: verb, File: program, Parameters: parameters, Directory: directory, Show: windows.SW_HIDE}
	if ok, _, err := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return 0, errors.New("Windows administrator prompt was cancelled. No service changes were made")
		}
		return 0, fmt.Errorf("launch service manager: %w", err)
	}
	if info.Process == 0 {
		return 0, errors.New("Windows did not return a setup process handle")
	}
	defer windows.CloseHandle(info.Process)
	for {
		result, err := windows.WaitForSingleObject(info.Process, 250)
		if err != nil {
			return 0, err
		}
		if result == windows.WAIT_OBJECT_0 {
			break
		}
		if ctx.Err() != nil {
			return 0, errors.New("desktop closed while the service operation was running; Windows will finish the operation")
		}
	}
	var code uint32
	err := windows.GetExitCodeProcess(info.Process, &code)
	return code, err
}

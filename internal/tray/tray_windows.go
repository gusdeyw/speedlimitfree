// Package tray provides the small Windows notification-area surface for the desktop.
// Its message loop sleeps in GetMessage; no background status polling is required.
package tray

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	windowClass   = "SpeedLimitFree.Tray"
	wmClose       = 0x0010
	wmDestroy     = 0x0002
	wmCommand     = 0x0111
	wmContextMenu = 0x007b
	wmTray        = 0x8001
	wmMenuReady   = 0x8002
	commandOpen   = 1001
	commandPause  = 1002
	commandQuit   = 1003
	commandResume = 1004
)

type State struct {
	Connected bool
	Paused    bool
	Status    string
}

type Options struct {
	IconPNG   []byte
	Open      func()
	SetPaused func(bool)
	Quit      func()
	Refresh   func() State
	Failed    func(error)
}

type Tray struct {
	opts           Options
	hwnd           atomic.Uintptr
	available      atomic.Bool
	closing        atomic.Bool
	menuPending    atomic.Bool
	mu             sync.Mutex
	menuState      State
	done           chan struct{}
	icon           uintptr // Owned by the tray's OS thread.
	callback       uintptr
	taskbarCreated uint32
}

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	shell32               = windows.NewLazySystemDLL("shell32.dll")
	registerClass         = user32.NewProc("RegisterClassExW")
	unregisterClass       = user32.NewProc("UnregisterClassW")
	createWindow          = user32.NewProc("CreateWindowExW")
	destroyWindow         = user32.NewProc("DestroyWindow")
	defWindowProc         = user32.NewProc("DefWindowProcW")
	getMessage            = user32.NewProc("GetMessageW")
	translateMessage      = user32.NewProc("TranslateMessage")
	dispatchMessage       = user32.NewProc("DispatchMessageW")
	postMessage           = user32.NewProc("PostMessageW")
	postQuitMessage       = user32.NewProc("PostQuitMessage")
	registerWindowMessage = user32.NewProc("RegisterWindowMessageW")
	createIcon            = user32.NewProc("CreateIconFromResourceEx")
	destroyIcon           = user32.NewProc("DestroyIcon")
	getSystemMetrics      = user32.NewProc("GetSystemMetrics")
	notifyIcon            = shell32.NewProc("Shell_NotifyIconW")
	createPopupMenu       = user32.NewProc("CreatePopupMenu")
	appendMenu            = user32.NewProc("AppendMenuW")
	destroyMenu           = user32.NewProc("DestroyMenu")
	getCursorPos          = user32.NewProc("GetCursorPos")
	setForegroundWindow   = user32.NewProc("SetForegroundWindow")
	trackPopupMenu        = user32.NewProc("TrackPopupMenu")
	endMenu               = user32.NewProc("EndMenu")
)

type point struct{ X, Y int32 }
type message struct {
	Hwnd           uintptr
	Message        uint32
	Wparam, Lparam uintptr
	Time           uint32
	Point          point
	Private        uint32
}
type wndClass struct {
	Size, Style                        uint32
	Callback                           uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}

// Timeout and Version share a union in the Windows ABI, so only one field exists.
type iconData struct {
	Size                       uint32
	Hwnd                       uintptr
	ID, Flags, CallbackMessage uint32
	Icon                       uintptr
	Tip                        [128]uint16
	State, StateMask           uint32
	Info                       [256]uint16
	Version                    uint32
	InfoTitle                  [64]uint16
	InfoFlags                  uint32
	GUID                       windows.GUID
	BalloonIcon                uintptr
}

func New(opts Options) (*Tray, error) {
	t := &Tray{opts: opts, done: make(chan struct{})}
	ready := make(chan error, 1)
	go t.run(ready)
	if err := <-ready; err != nil {
		<-t.done
		return nil, err
	}
	return t, nil
}
func (t *Tray) Available() bool { return t.available.Load() && !t.closing.Load() }
func (t *Tray) Close() {
	if t.closing.CompareAndSwap(false, true) {
		if h := t.hwnd.Load(); h != 0 {
			postMessage.Call(h, wmClose, 0, 0)
		}
	}
	<-t.done
}

func (t *Tray) run(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(t.done)
	defer t.available.Store(false)
	var instance windows.Handle
	if err := windows.GetModuleHandleEx(2, nil, &instance); err != nil {
		ready <- err
		return
	}
	className, _ := windows.UTF16PtrFromString(windowClass)
	t.callback = windows.NewCallback(t.windowProc)
	wc := wndClass{Size: uint32(unsafe.Sizeof(wndClass{})), Instance: uintptr(instance), Callback: t.callback, ClassName: className}
	if ok, _, err := registerClass.Call(uintptr(unsafe.Pointer(&wc))); ok == 0 {
		ready <- fmt.Errorf("register tray window: %w", err)
		return
	}
	defer unregisterClass.Call(uintptr(unsafe.Pointer(className)), uintptr(instance))
	h, _, err := createWindow.Call(0x80, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, uintptr(instance), 0)
	if h == 0 {
		ready <- fmt.Errorf("create tray window: %w", err)
		return
	}
	t.hwnd.Store(h)
	defer func() {
		t.removeIcon()
		destroyWindow.Call(h)
		if t.icon != 0 {
			destroyIcon.Call(t.icon)
		}
		t.hwnd.Store(0)
	}()
	if len(t.opts.IconPNG) == 0 {
		ready <- fmt.Errorf("tray icon is empty")
		return
	}
	w, _, _ := getSystemMetrics.Call(49)
	height, _, _ := getSystemMetrics.Call(50)
	t.icon, _, err = createIcon.Call(uintptr(unsafe.Pointer(&t.opts.IconPNG[0])), uintptr(len(t.opts.IconPNG)), 1, 0x30000, w, height, 0)
	if t.icon == 0 {
		ready <- fmt.Errorf("load tray icon: %w", err)
		return
	}
	taskbarName, _ := windows.UTF16PtrFromString("TaskbarCreated")
	registered, _, err := registerWindowMessage.Call(uintptr(unsafe.Pointer(taskbarName)))
	if registered == 0 {
		ready <- fmt.Errorf("register Explorer recovery: %w", err)
		return
	}
	t.taskbarCreated = uint32(registered)
	if err = t.addIcon(); err != nil {
		ready <- err
		return
	}
	ready <- nil
	for {
		var m message
		result, _, err := getMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(result) == -1 {
			t.fail(fmt.Errorf("tray message loop: %w", err))
			return
		}
		if result == 0 {
			return
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&m)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (t *Tray) data() iconData {
	return iconData{Size: uint32(unsafe.Sizeof(iconData{})), Hwnd: t.hwnd.Load(), ID: 1}
}
func (t *Tray) addIcon() error {
	n := t.data()
	n.Flags = 1 | 2 | 4 | 0x80
	n.CallbackMessage = wmTray
	n.Icon = t.icon
	copy(n.Tip[:], windows.StringToUTF16("SpeedLimitFree — click to open"))
	if ok, _, err := notifyIcon.Call(0, uintptr(unsafe.Pointer(&n))); ok == 0 {
		return fmt.Errorf("add tray icon: %w", err)
	}
	n.Version = 4
	notifyIcon.Call(4, uintptr(unsafe.Pointer(&n)))
	t.available.Store(true)
	return nil
}
func (t *Tray) removeIcon() {
	n := t.data()
	if n.Hwnd != 0 {
		notifyIcon.Call(2, uintptr(unsafe.Pointer(&n)))
	}
	t.available.Store(false)
}
func (t *Tray) fail(err error) {
	t.available.Store(false)
	if t.opts.Failed != nil {
		go t.opts.Failed(err)
	}
}

func (t *Tray) windowProc(h uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmClose:
		endMenu.Call()
		t.removeIcon()
		postQuitMessage.Call(0)
		return 0
	case wmDestroy:
		postQuitMessage.Call(0)
		return 0
	case wmCommand:
		t.command(uint32(wparam & 0xffff))
		return 0
	case wmMenuReady:
		t.mu.Lock()
		state := t.menuState
		t.mu.Unlock()
		t.showMenu(state)
		t.menuPending.Store(false)
		return 0
	case wmTray:
		switch uint32(lparam & 0xffff) {
		case 0x400, 0x401, 0x202, 0x203:
			if t.opts.Open != nil {
				go t.opts.Open()
			}
		case wmContextMenu, 0x205:
			t.requestMenu()
		}
		return 0
	default:
		if msg == t.taskbarCreated && t.taskbarCreated != 0 && !t.closing.Load() {
			if err := t.addIcon(); err != nil {
				t.fail(err)
			}
			return 0
		}
	}
	result, _, _ := defWindowProc.Call(h, uintptr(msg), wparam, lparam)
	return result
}

func (t *Tray) requestMenu() {
	if t.closing.Load() || !t.menuPending.CompareAndSwap(false, true) {
		return
	}
	go func() {
		state := State{Status: "Service offline"}
		if t.opts.Refresh != nil {
			state = t.opts.Refresh()
		}
		t.mu.Lock()
		t.menuState = state
		t.mu.Unlock()
		if t.closing.Load() {
			t.menuPending.Store(false)
			return
		}
		if ok, _, _ := postMessage.Call(t.hwnd.Load(), wmMenuReady, 0, 0); ok == 0 {
			t.menuPending.Store(false)
		}
	}()
}

func (t *Tray) showMenu(state State) {
	h := t.hwnd.Load()
	menu, _, err := createPopupMenu.Call()
	if menu == 0 {
		if t.opts.Failed != nil {
			go t.opts.Failed(fmt.Errorf("open tray menu: %w", err))
		}
		return
	}
	defer destroyMenu.Call(menu)
	add := func(id uintptr, label string, disabled bool) {
		text, _ := windows.UTF16PtrFromString(label)
		flags := uintptr(0)
		if disabled {
			flags = 1
		}
		appendMenu.Call(menu, flags, id, uintptr(unsafe.Pointer(text)))
	}
	add(commandOpen, "Open SpeedLimitFree", false)
	appendMenu.Call(menu, 0x800, 0, 0)
	add(0, state.Status, true)
	if state.Paused {
		add(commandResume, "Resume limits", !state.Connected)
	} else {
		add(commandPause, "Pause limits", !state.Connected)
	}
	appendMenu.Call(menu, 0x800, 0, 0)
	add(0, "Quitting keeps the service running", true)
	add(commandQuit, "Quit SpeedLimitFree", false)
	var position point
	getCursorPos.Call(uintptr(unsafe.Pointer(&position)))
	setForegroundWindow.Call(h)
	selected, _, _ := trackPopupMenu.Call(menu, 0x100|0x2, uintptr(position.X), uintptr(position.Y), 0, h, 0)
	postMessage.Call(h, 0, 0, 0)
	if selected != 0 {
		t.command(uint32(selected))
	}
}

func (t *Tray) command(id uint32) {
	if t.closing.Load() {
		return
	}
	switch id {
	case commandOpen:
		if t.opts.Open != nil {
			go t.opts.Open()
		}
	case commandPause:
		if t.opts.SetPaused != nil {
			go t.opts.SetPaused(true)
		}
	case commandResume:
		if t.opts.SetPaused != nil {
			go t.opts.SetPaused(false)
		}
	case commandQuit:
		if t.opts.Quit != nil {
			go t.opts.Quit()
		}
	}
}

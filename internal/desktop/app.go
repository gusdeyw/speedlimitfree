package desktop

import (
	"context"
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"os"
	"path/filepath"
	"speedlimitfree/internal/appicons"
	"speedlimitfree/internal/contracts"
	"speedlimitfree/internal/ipc"
	"speedlimitfree/internal/processes"
	"speedlimitfree/internal/servicectl"
	"speedlimitfree/internal/tray"
	"sync"
	"sync/atomic"
	"time"
)

type App struct {
	ctx          context.Context
	cancel       context.CancelFunc
	visible      atomic.Bool
	hiddenToTray atomic.Bool
	quitting     atomic.Bool
	tray         atomic.Pointer[tray.Tray]
	trayIcon     []byte
	serviceBusy  atomic.Bool
	icons        appicons.Cache
	cacheMu      sync.Mutex
	cacheAt      time.Time
	cache        []contracts.Process
}

func New(icon []byte) *App { return &App{trayIcon: icon} }
func (a *App) Startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.visible.Store(true)
	t, err := tray.New(tray.Options{IconPNG: a.trayIcon, Open: a.Show, Quit: a.Quit, SetPaused: a.traySetPaused, Refresh: a.trayState, Failed: a.trayError})
	if err != nil {
		runtime.LogError(a.ctx, err.Error())
	} else {
		a.tray.Store(t)
	}
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				if a.visible.Load() && !a.hiddenToTray.Load() && !runtime.WindowIsMinimised(a.ctx) {
					s := a.Snapshot()
					if a.visible.Load() && !a.hiddenToTray.Load() {
						runtime.EventsEmit(a.ctx, "snapshot", s)
					}
				}
			}
		}
	}()
}
func (a *App) Shutdown(context.Context) {
	a.quitting.Store(true)
	if a.cancel != nil {
		a.cancel()
	}
	if t := a.tray.Load(); t != nil {
		t.Close()
	}
}
func (a *App) SetVisible(visible bool) { a.visible.Store(visible && !a.hiddenToTray.Load()) }

// BeforeClose prevents a title-bar close only when a working tray can restore the UI.
func (a *App) BeforeClose(ctx context.Context) bool {
	if a.quitting.Load() {
		return false
	}
	if t := a.tray.Load(); t != nil && t.Available() {
		a.hiddenToTray.Store(true)
		a.visible.Store(false)
		runtime.WindowHide(ctx)
		return true
	}
	return false
}
func (a *App) Show() {
	if a.ctx == nil || a.quitting.Load() {
		return
	}
	a.hiddenToTray.Store(false)
	a.visible.Store(true)
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
}

// Quit exits the UI and tray. Service lifetime is independent of the desktop.
func (a *App) Quit() {
	if a.quitting.CompareAndSwap(false, true) {
		runtime.Quit(a.ctx)
	}
}
func (a *App) trayState() tray.State {
	ctx, cancel := context.WithTimeout(a.ctx, 750*time.Millisecond)
	defer cancel()
	s, err := ipc.Call(ctx, contracts.Request{Method: "snapshot"})
	if err != nil {
		return tray.State{Status: "Service offline"}
	}
	status := "Limits active"
	if s.Engine != "running" {
		status = "Traffic engine inactive"
	}
	if s.Paused {
		status = "Limits paused"
	}
	return tray.State{Connected: s.Connected, Paused: s.Paused, Status: status}
}
func (a *App) traySetPaused(paused bool) {
	s, err := a.SetPaused(paused)
	if err != nil {
		a.trayError(err)
		return
	}
	if a.visible.Load() && !a.hiddenToTray.Load() {
		runtime.EventsEmit(a.ctx, "snapshot", s)
	}
}
func (a *App) trayError(err error) {
	if a.quitting.Load() {
		return
	}
	runtime.LogError(a.ctx, err.Error())
	a.Show()
	runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{Type: runtime.ErrorDialog, Title: "SpeedLimitFree", Message: err.Error()})
}
func (a *App) Snapshot() contracts.Snapshot {
	s, err := ipc.Call(a.ctx, contracts.Request{Method: "snapshot"})
	if err == nil {
		return s
	}
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	if time.Since(a.cacheAt) > 2*time.Second {
		a.cache, _ = processes.List()
		a.cacheAt = time.Now()
	}
	ps := a.cache
	if ps == nil {
		ps = []contracts.Process{}
	}
	return contracts.Snapshot{Version: contracts.Version, Engine: "offline", Message: "Install or start the background service to measure traffic and apply limits.", Processes: ps, Rules: []contracts.Rule{}}
}
func (a *App) SaveRule(rule contracts.Rule) (contracts.Snapshot, error) {
	return ipc.Call(a.ctx, contracts.Request{Method: "save", Rule: &rule})
}
func (a *App) DeleteRule(id string) (contracts.Snapshot, error) {
	return ipc.Call(a.ctx, contracts.Request{Method: "delete", ID: id})
}
func (a *App) SetPaused(paused bool) (contracts.Snapshot, error) {
	return ipc.Call(a.ctx, contracts.Request{Method: "pause", Paused: paused})
}
func (a *App) ChooseExecutable() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Choose an application", Filters: []runtime.FileFilter{{DisplayName: "Windows applications", Pattern: "*.exe"}}})
}
func (a *App) ServiceStatus() (contracts.ServiceStatus, error) { return servicectl.Status() }

func (a *App) AppIcon(path string) string { return a.icons.Get(path) }

func (a *App) ManageService(action string) (contracts.ServiceResult, error) {
	if !a.serviceBusy.CompareAndSwap(false, true) {
		return contracts.ServiceResult{}, errors.New("a service operation is already running")
	}
	defer a.serviceBusy.Store(false)
	exe, err := os.Executable()
	if err != nil {
		return contracts.ServiceResult{}, err
	}
	return servicectl.Manage(a.ctx, action, filepath.Dir(exe))
}

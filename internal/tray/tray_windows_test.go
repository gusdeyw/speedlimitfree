package tray

import (
	"os"
	"testing"
	"time"
	"unsafe"
)

func TestWindowsABILayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("Windows x64 is the supported target")
	}
	if size := unsafe.Sizeof(iconData{}); size != 976 {
		t.Fatalf("NOTIFYICONDATAW is %d bytes, expected 976", size)
	}
	if offset := unsafe.Offsetof(iconData{}.InfoTitle); offset != 820 {
		t.Fatalf("NOTIFYICONDATAW union layout differs: InfoTitle offset %d", offset)
	}
	if unsafe.Sizeof(wndClass{}) != 80 || unsafe.Sizeof(message{}) != 48 {
		t.Fatal("Windows message/class layout differs")
	}
}

func TestNativeTrayActionsAndCleanup(t *testing.T) {
	icon, err := os.ReadFile("../../build/appicon.png")
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan string, 8)
	tr, err := New(Options{IconPNG: icon, Open: func() { events <- "open" }, SetPaused: func(paused bool) {
		if paused {
			events <- "pause"
		} else {
			events <- "resume"
		}
	}, Quit: func() { events <- "quit" }})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	if !tr.Available() {
		t.Fatal("native notification icon not registered")
	}
	for _, item := range []struct {
		id    uint32
		event string
	}{{commandOpen, "open"}, {commandPause, "pause"}, {commandResume, "resume"}, {commandQuit, "quit"}} {
		if ok, _, err := postMessage.Call(tr.hwnd.Load(), wmCommand, uintptr(item.id), 0); ok == 0 {
			t.Fatal(err)
		}
		select {
		case event := <-events:
			if event != item.event {
				t.Fatalf("expected %s, got %s", item.event, event)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("native tray command timed out")
		}
	}
	h := tr.hwnd.Load()
	tr.Close()
	if tr.Available() {
		t.Fatal("closed tray is still available")
	}
	if ok, _, _ := user32.NewProc("IsWindow").Call(h); ok != 0 {
		t.Fatal("tray window not destroyed")
	}
}

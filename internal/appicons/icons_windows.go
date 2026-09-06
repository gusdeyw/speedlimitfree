// Package appicons reads Windows executable icons without creating any image files.
package appicons

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type entry struct {
	data    string
	expires time.Time
	used    time.Time
}
type Cache struct {
	mu      sync.Mutex
	entries map[string]entry
}

// Get returns a small PNG data URI, or empty for inaccessible/unsupported files.
// Calls are serialized off the UI thread. The bounded cache includes failed lookups.
func (c *Cache) Get(path string) string {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) || len(filepath.VolumeName(path)) != 2 || !strings.EqualFold(filepath.Ext(path), ".exe") {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]entry)
	}
	key, now := strings.ToLower(path), time.Now()
	if e, ok := c.entries[key]; ok && now.Before(e.expires) {
		e.used = now
		c.entries[key] = e
		return e.data
	}
	root, _ := windows.UTF16PtrFromString(filepath.VolumeName(path) + `\`)
	// Never access remote drives while discovering icons.
	if windows.GetDriveType(root) == windows.DRIVE_REMOTE {
		return ""
	}
	data := ""
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		if pngData, err := extract(path); err == nil {
			data = "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngData)
		}
	}
	if len(c.entries) >= 256 {
		var oldest string
		var used time.Time
		for k, e := range c.entries {
			if oldest == "" || e.used.Before(used) {
				oldest, used = k, e.used
			}
		}
		delete(c.entries, oldest)
	}
	ttl := 5 * time.Minute
	if data == "" {
		ttl = 30 * time.Second
	}
	c.entries[key] = entry{data: data, expires: now.Add(ttl), used: now}
	return data
}

var (
	shell = windows.NewLazySystemDLL("shell32.dll")
	user  = windows.NewLazySystemDLL("user32.dll")
	gdi   = windows.NewLazySystemDLL("gdi32.dll")
)

type fileInfo struct {
	Icon        uintptr
	Index       int32
	Attributes  uint32
	DisplayName [260]uint16
	TypeName    [80]uint16
}
type bitmapHeader struct {
	Size                   uint32
	Width, Height          int32
	Planes, BitCount       uint16
	Compression, SizeImage uint32
	XPels, YPels           int32
	ClrUsed, ClrImportant  uint32
}

func extract(path string) ([]byte, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ole := windows.NewLazySystemDLL("ole32.dll")
	hr, _, _ := ole.NewProc("CoInitializeEx").Call(0, 2)
	if int32(hr) < 0 {
		return nil, fmt.Errorf("initialize icon thread: %x", hr)
	}
	defer ole.NewProc("CoUninitialize").Call()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var info fileInfo
	ok, _, err := shell.NewProc("SHGetFileInfoW").Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), 0x100)
	if ok == 0 || info.Icon == 0 {
		return nil, fmt.Errorf("read executable icon: %w", err)
	}
	defer user.NewProc("DestroyIcon").Call(info.Icon)
	black, err := draw(info.Icon, 0)
	if err != nil {
		return nil, err
	}
	white, err := draw(info.Icon, 255)
	if err != nil {
		return nil, err
	}
	// Draw against black and white to recover alpha for both modern and mask icons.
	out := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for i := 0; i < len(black); i += 4 {
		delta := 0
		for j := 0; j < 3; j++ {
			if d := int(white[i+j]) - int(black[i+j]); d > delta {
				delta = d
			}
		}
		a := 255 - delta
		if a <= 0 {
			continue
		}
		out.Pix[i+3] = uint8(a)
		for j := 0; j < 3; j++ {
			v := (int(black[i+2-j])*255 + a/2) / a
			if v > 255 {
				v = 255
			}
			out.Pix[i+j] = uint8(v)
		}
	}
	var buf bytes.Buffer
	if err = png.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func draw(icon uintptr, background byte) ([]byte, error) {
	dc, _, err := gdi.NewProc("CreateCompatibleDC").Call(0)
	if dc == 0 {
		return nil, err
	}
	defer gdi.NewProc("DeleteDC").Call(dc)
	header := bitmapHeader{Size: 40, Width: 32, Height: -32, Planes: 1, BitCount: 32}
	var bits *byte
	bmp, _, err := gdi.NewProc("CreateDIBSection").Call(dc, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		return nil, err
	}
	defer gdi.NewProc("DeleteObject").Call(bmp)
	old, _, err := gdi.NewProc("SelectObject").Call(dc, bmp)
	if old == 0 || old == ^uintptr(0) {
		return nil, err
	}
	defer gdi.NewProc("SelectObject").Call(dc, old)
	pixels := unsafe.Slice(bits, 32*32*4)
	for i := range pixels {
		pixels[i] = background
	}
	ok, _, err := user.NewProc("DrawIconEx").Call(dc, 0, 0, icon, 32, 32, 0, 0, 3)
	if ok == 0 {
		return nil, err
	}
	gdi.NewProc("GdiFlush").Call()
	return append([]byte(nil), pixels...), nil
}

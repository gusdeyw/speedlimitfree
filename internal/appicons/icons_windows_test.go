package appicons

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestExecutableIconAndHandleCleanup(t *testing.T) {
	path := filepath.Join(os.Getenv("SystemRoot"), "explorer.exe")
	data, err := extract(path)
	if err != nil {
		t.Fatal(err)
	}
	im, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if im.Bounds().Dx() != 32 || im.Bounds().Dy() != 32 {
		t.Fatal("unexpected icon dimensions")
	}
	transparent, visible := 0, 0
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			_, _, _, a := im.At(x, y).RGBA()
			if a == 0 {
				transparent++
			} else {
				visible++
			}
		}
	}
	if transparent == 0 || visible == 0 {
		t.Fatal("icon transparency/content was lost")
	}
	resources := user.NewProc("GetGuiResources")
	before, _, _ := resources.Call(uintptr(windows.CurrentProcess()), 0)
	usersBefore, _, _ := resources.Call(uintptr(windows.CurrentProcess()), 1)
	for i := 0; i < 12; i++ {
		if _, err = extract(path); err != nil {
			t.Fatal(err)
		}
	}
	after, _, _ := resources.Call(uintptr(windows.CurrentProcess()), 0)
	usersAfter, _, _ := resources.Call(uintptr(windows.CurrentProcess()), 1)
	if after > before+4 || usersAfter > usersBefore+4 {
		t.Fatalf("native handles grew: GDI %d->%d, USER %d->%d", before, after, usersBefore, usersAfter)
	}
}

func TestMemoryCacheBoundedAndPathValidation(t *testing.T) {
	var c Cache
	for _, path := range []string{`\\server\share\app.exe`, `relative.exe`, `C:\Windows\not-an-exe.txt`, ""} {
		if c.Get(path) != "" {
			t.Fatal("unsupported path returned an icon")
		}
	}
	path := filepath.Join(os.Getenv("SystemRoot"), "explorer.exe")
	data := c.Get(path)
	if !strings.HasPrefix(data, "data:image/png;base64,") {
		t.Fatal("missing PNG data URI")
	}
	if _, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, "data:image/png;base64,")); err != nil {
		t.Fatal(err)
	}
	if c.Get(strings.ToUpper(path)) != data || len(c.entries) != 1 {
		t.Fatal("same executable was not cached")
	}
	dir := t.TempDir()
	for i := 0; i < 270; i++ {
		c.Get(filepath.Join(dir, fmt.Sprintf("missing-%d.exe", i)))
	}
	if len(c.entries) > 256 {
		t.Fatal("icon cache exceeded its bound")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatal("icon lookup created files")
	}
}

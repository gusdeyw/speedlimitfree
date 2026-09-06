package servicectl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestSetupResultRequiresMatchingSuccessfulExit(t *testing.T) {
	good := `{"operationId":"current","action":"start","success":true,"message":"Running"}`
	for _, tc := range []struct {
		name, body, id string
		code           uint32
		wantErr        bool
	}{
		{"success", good, "current", 0, false},
		{"powershell BOM", "\xef\xbb\xbf" + good, "current", 0, false},
		{"stale result", good, "other", 0, true},
		{"failed process", good, "current", 1, true},
		{"reported failure", `{"operationId":"current","action":"start","success":false,"message":"Access denied"}`, "current", 0, true},
		{"incomplete result", `{}`, "current", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeResult([]byte(tc.body), tc.id, "start", tc.code)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
}

func TestDiagnosticLogsAreBounded(t *testing.T) {
	p := filepath.Join(t.TempDir(), "service.log")
	if err := os.WriteFile(p, []byte(strings.Repeat("x", 20000)+"latest error"), 0600); err != nil {
		t.Fatal(err)
	}
	got := tail(p)
	if len(got) > 8192 || !strings.HasSuffix(got, "latest error") {
		t.Fatal("log tail did not preserve latest bounded diagnostic")
	}
}

func TestShellExecuteInfoLayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) == 8 && (unsafe.Sizeof(shellExecuteInfo{}) != 112 || unsafe.Offsetof(shellExecuteInfo{}.Process) != 104) {
		t.Fatal("invalid SHELLEXECUTEINFOW x64 ABI")
	}
}

func TestReadWindowsServiceWithoutElevation(t *testing.T) {
	s, err := Status()
	if err != nil {
		t.Fatal(err)
	}
	if s.State == "" || s.LogPath == "" {
		t.Fatal("missing service diagnostics")
	}
}

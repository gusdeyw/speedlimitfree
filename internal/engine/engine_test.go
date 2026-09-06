package engine

import (
	"os"
	"path/filepath"
	"speedlimitfree/internal/contracts"
	"speedlimitfree/internal/processes"
	"testing"
)

func TestCommandsCommitOnlyAfterPersistence(t *testing.T) {
	dir := t.TempDir()
	e, err := New(filepath.Join(dir, "rules.json"))
	if err != nil {
		t.Fatal(err)
	}
	r := contracts.Rule{Scope: "application", Path: `C:\App\app.exe`, Name: "App", Enabled: true}
	if err = e.Command(contracts.Request{Method: "save", Rule: &r}); err != nil {
		t.Fatal(err)
	}
	s := e.Snapshot()
	if len(s.Rules) != 1 || s.Rules[0].ID == "" {
		t.Fatal("rule missing")
	}
	if err = e.Command(contracts.Request{Method: "save", Rule: &r}); err == nil {
		t.Fatal("duplicate target allowed")
	}
	e.path = filepath.Join(dir, "invalid", "rules.json")
	os.WriteFile(filepath.Join(dir, "invalid"), []byte("file"), 0600)
	if err = e.Command(contracts.Request{Method: "pause", Paused: true}); err == nil {
		t.Fatal("expected write failure")
	}
	if e.Snapshot().Paused {
		t.Fatal("failed change was applied")
	}
}
func TestRejectReusedOrMissingProcess(t *testing.T) {
	e, err := New(filepath.Join(t.TempDir(), "rules.json"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := processes.Get(uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	r := contracts.Rule{Scope: "process", Path: p.Path, Name: p.Name, PID: p.PID, Started: p.Started, Enabled: true}
	if err = e.Command(contracts.Request{Method: "save", Rule: &r}); err != nil {
		t.Fatal(err)
	}
	r.Started = "different"
	if err = e.Command(contracts.Request{Method: "save", Rule: &r}); err == nil {
		t.Fatal("stale identity accepted")
	}
}

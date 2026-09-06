package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

func TestInvalidSavedRulesNeverReportsRunning(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(`invalid json`), 0600); err != nil {
		t.Fatal(err)
	}
	s := service{data: dir, monitor: true}
	states := make(chan svc.Status, 10)
	done := make(chan uint32, 1)
	go func() { _, code := s.Execute(nil, make(chan svc.ChangeRequest), states); done <- code }()
	select {
	case code := <-done:
		if code == 0 {
			t.Fatal("invalid settings reported success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("startup did not finish")
	}
	close(states)
	for state := range states {
		if state.State == svc.Running {
			t.Fatal("SCM was told Running before initialization succeeded")
		}
	}
}

package ipc

import (
	"context"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"speedlimitfree/internal/contracts"
	"speedlimitfree/internal/engine"
	"testing"
	"time"
)

func TestNamedPipeCommandsAndShutdown(t *testing.T) {
	previous := contracts.PipeName
	contracts.PipeName = fmt.Sprintf(`\\.\pipe\SpeedLimitFree.test.%d`, os.Getpid())
	defer func() { contracts.PipeName = previous }()
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.New(filepath.Join(t.TempDir(), "rules.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, u.User.Sid.String(), e) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(7 * time.Second):
			t.Error("pipe server did not stop")
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		probe, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_, err = Call(probe, contracts.Request{Method: "snapshot"})
		stop()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	r := contracts.Rule{Scope: "application", Path: `C:\Apps\Test.exe`, Name: "Test", Enabled: true}
	s, err := Call(ctx, contracts.Request{Method: "save", Rule: &r})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Rules) != 1 {
		t.Fatal("rule not returned")
	}
	s, err = Call(ctx, contracts.Request{Method: "pause", Paused: true})
	if err != nil || !s.Paused {
		t.Fatal("pause not acknowledged")
	}
	if _, err = Call(ctx, contracts.Request{Method: "unknown"}); err == nil {
		t.Fatal("invalid command accepted")
	}
	if _, err = Call(ctx, contracts.Request{Method: "delete", ID: s.Rules[0].ID}); err != nil {
		t.Fatal(err)
	}
}

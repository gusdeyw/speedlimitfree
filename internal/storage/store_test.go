package storage

import (
	"os"
	"path/filepath"
	"speedlimitfree/internal/contracts"
	"testing"
)

func TestPersistenceAndTemporaryRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	rate := int64(5000000)
	s := contracts.Settings{Paused: true, Rules: []contracts.Rule{{ID: "app", Scope: "application", Name: "App", Path: `C:\Apps\App.exe`, Download: &rate, Enabled: true}, {ID: "proc", Scope: "process", Path: `C:\Apps\Other.exe`, PID: 42, Started: "100", Enabled: true}}}
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Paused || len(got.Rules) != 1 || *got.Rules[0].Download != rate {
		t.Fatalf("incorrect roundtrip: %+v", got)
	}
	s.Paused = false
	if err = Save(path, s); err != nil {
		t.Fatal(err)
	}
	got, err = Load(path)
	if err != nil || got.Paused {
		t.Fatal("atomic replacement failed")
	}
}
func TestCorruptSettingsAreNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	original := []byte(`{"version":99,"rules":[]}`)
	os.WriteFile(path, original, 0600)
	if _, err := Load(path); err == nil {
		t.Fatal("unknown version accepted")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Fatal("invalid settings modified")
	}
}

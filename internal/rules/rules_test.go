package rules

import (
	"speedlimitfree/internal/contracts"
	"testing"
)

func TestIdentityPrecedenceAndDisabledOverrides(t *testing.T) {
	p := contracts.Process{PID: 42, Started: "100", Path: `C:\Apps\Browser.exe`}
	rs := []contracts.Rule{{ID: "app", Scope: "application", Path: `c:\apps\browser.EXE`, Enabled: true}, {ID: "process", Scope: "process", Path: p.Path, PID: 42, Started: "100", Enabled: true}}
	if Match(rs, p).ID != "process" {
		t.Fatal("process rule did not override application")
	}
	p.Started = "101"
	if Match(rs, p).ID != "app" {
		t.Fatal("PID reuse matched expired rule")
	}
	p.Started = "100"
	rs[1].Enabled = false
	if Match(rs, p).ID != "app" {
		t.Fatal("disabled override hid application rule")
	}
	p.Path = `C:\Other\Browser.exe`
	if Match(rs, p) != nil {
		t.Fatal("matched basename rather than path")
	}
}
func TestLimitValidation(t *testing.T) {
	zero := int64(0)
	negative := int64(-1)
	r := contracts.Rule{Scope: "application", Path: `C:\Apps\App.exe`}
	if err := Validate(r); err != nil {
		t.Fatal(err)
	}
	for _, n := range []*int64{&zero, &negative} {
		r.Download = n
		if Validate(r) == nil {
			t.Fatal("invalid rate accepted")
		}
	}
	r.Download = nil
	r.Scope = "process"
	r.PID = 44
	if Validate(r) == nil {
		t.Fatal("missing creation time accepted")
	}
}

package rules

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"speedlimitfree/internal/contracts"
	"strings"
)

func Path(s string) string                { return strings.ToLower(filepath.Clean(s)) }
func Identity(p contracts.Process) string { return fmt.Sprintf("%d:%s", p.PID, p.Started) }

func Validate(r contracts.Rule) error {
	if r.Scope != "application" && r.Scope != "process" {
		return errors.New("choose application or process scope")
	}
	if r.Path == "" || !filepath.IsAbs(r.Path) || !strings.EqualFold(filepath.Ext(r.Path), ".exe") {
		return errors.New("an absolute executable path is required")
	}
	if len(r.Path) > 32767 || len(r.Name) > 160 || len(r.ID) > 64 {
		return errors.New("rule text is too long")
	}
	if r.Scope == "process" && (r.PID == 0 || r.Started == "") {
		return errors.New("process identity is required")
	}
	for _, rate := range []*int64{r.Download, r.Upload} {
		if rate != nil && (*rate < 1 || *rate > 125_000_000_000) {
			return errors.New("limits must be between 1 byte/s and 125 GB/s, or unlimited")
		}
	}
	return nil
}

func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func Match(rs []contracts.Rule, p contracts.Process) *contracts.Rule {
	var application *contracts.Rule
	for i := range rs {
		r := &rs[i]
		if !r.Enabled || Path(r.Path) != Path(p.Path) {
			continue
		}
		if r.Scope == "process" && r.PID == p.PID && r.Started == p.Started {
			return r
		}
		if r.Scope == "application" {
			application = r
		}
	}
	return application
}

func SameTarget(a, b contracts.Rule) bool {
	return a.Scope == b.Scope && Path(a.Path) == Path(b.Path) && (a.Scope == "application" || (a.PID == b.PID && a.Started == b.Started))
}

// Index keeps packet matching independent of the number of saved rules.
type processKey struct {
	PID     uint32
	Started string
}
type Index struct {
	applications map[string]*contracts.Rule
	processes    map[processKey]*contracts.Rule
}

func Compile(rs []contracts.Rule) *Index {
	i := &Index{applications: map[string]*contracts.Rule{}, processes: map[processKey]*contracts.Rule{}}
	for _, r := range rs {
		if !r.Enabled {
			continue
		}
		copy := r
		if r.Scope == "application" {
			i.applications[Path(r.Path)] = &copy
		} else {
			i.processes[processKey{r.PID, r.Started}] = &copy
		}
	}
	return i
}
func (i *Index) Lookup(p contracts.Process) *contracts.Rule {
	if r := i.processes[processKey{p.PID, p.Started}]; r != nil && Path(r.Path) == Path(p.Path) {
		return r
	}
	if len(i.applications) == 0 {
		return nil
	}
	return i.applications[Path(p.Path)]
}

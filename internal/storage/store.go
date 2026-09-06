package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"speedlimitfree/internal/contracts"
	"speedlimitfree/internal/rules"
)

func Load(path string) (contracts.Settings, error) {
	s := contracts.Settings{Version: contracts.Version, Rules: []contracts.Rule{}}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	d.DisallowUnknownFields()
	if err = d.Decode(&s); err != nil {
		return s, fmt.Errorf("read rules: %w", err)
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return s, errors.New("unexpected trailing rules data")
	}
	if s.Version != contracts.Version {
		return s, errors.New("unsupported rules version")
	}
	if len(s.Rules) > 1000 {
		return s, errors.New("too many rules")
	}
	ids := map[string]bool{}
	for i, r := range s.Rules {
		if err := rules.Validate(r); err != nil {
			return s, err
		}
		if r.ID == "" || ids[r.ID] {
			return s, errors.New("invalid or duplicate rule ID")
		}
		ids[r.ID] = true
		for _, other := range s.Rules[:i] {
			if rules.SameTarget(r, other) {
				return s, errors.New("duplicate rule target")
			}
		}
	}
	// Process overrides belong only to the lifetime of the service session.
	app := make([]contracts.Rule, 0, len(s.Rules))
	for _, r := range s.Rules {
		if r.Scope == "application" {
			app = append(app, r)
		}
	}
	s.Rules = app
	return s, nil
}

func Save(path string, s contracts.Settings) error {
	s.Version = contracts.Version
	app := make([]contracts.Rule, 0, len(s.Rules))
	for _, r := range s.Rules {
		if r.Scope == "application" {
			app = append(app, r)
		}
	}
	s.Rules = app
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".rules-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return replace(tmp, path)
}

// Package state loads and saves the daemon's lease table (state.json). Writes are
// atomic (temp file + rename) so a crash mid-write never corrupts the table. The
// on-disk shape is SPEC.md §9.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Version is the current state.json schema version.
const Version = 1

// Instance is one leased checkout's record.
type Instance struct {
	Project    string         `json:"project"`
	Label      string         `json:"label"`
	Block      [2]int         `json:"block"`  // inclusive [lo, hi]
	Berths     map[string]int `json:"berths"` // service -> port, includes "tilt"
	CreatedAt  time.Time      `json:"createdAt"`
	LastSeenAt time.Time      `json:"lastSeenAt"`
}

// State is the full lease table keyed by instance (absolute checkout) path.
type State struct {
	Version   int                  `json:"version"`
	Instances map[string]*Instance `json:"instances"`
}

// New returns an empty, current-version state.
func New() *State {
	return &State{Version: Version, Instances: map[string]*Instance{}}
}

// Load reads state from path. A missing file yields a fresh empty state (not an
// error) — the first run has no table yet.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if s.Instances == nil {
		s.Instances = map[string]*Instance{}
	}
	if s.Version == 0 {
		s.Version = Version
	}
	return &s, nil
}

// Save writes state to path atomically: it marshals to a temp file in the same
// directory, fsyncs, and renames over the target, so readers never see a partial
// file. Parent directories are created as needed.
func Save(path string, s *State) error {
	if s.Version == 0 {
		s.Version = Version
	}
	if s.Instances == nil {
		s.Instances = map[string]*Instance{}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // harmless no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

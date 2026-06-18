// Package config resolves harbormaster's XDG paths and loads its two config
// files: the machine-wide global config (config.toml) and the per-project config
// (harbormaster.toml at a repo root). See SPEC.md §7.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Global is the machine-wide configuration. Field defaults come from
// DefaultGlobal; a config.toml overlays only the keys it sets.
type Global struct {
	PoolStart int    `toml:"pool_start"`
	PoolEnd   int    `toml:"pool_end"` // half-open: ports run [PoolStart, PoolEnd)
	BlockSize int    `toml:"block_size"`
	Reserved  []int  `toml:"reserved"`
	Socket    string `toml:"socket"`
	State     string `toml:"state"`
}

// DefaultGlobal returns the built-in defaults with XDG-resolved socket/state
// paths. Per SPEC §4/§7: pool [20000, 32000), block size 20.
func DefaultGlobal() Global {
	return Global{
		PoolStart: 20000,
		PoolEnd:   32000,
		BlockSize: 20,
		Reserved:  nil,
		Socket:    defaultSocket(),
		State:     defaultState(),
	}
}

// LoadGlobal loads the global config from its default path, falling back to
// DefaultGlobal when the file is absent.
func LoadGlobal() (Global, error) {
	return LoadGlobalFrom(GlobalPath())
}

// LoadGlobalFrom loads the global config from path. A missing file is not an
// error: the defaults are returned. Present keys override defaults; absent keys
// keep them.
func LoadGlobalFrom(path string) (Global, error) {
	g := DefaultGlobal()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return g, nil
	}
	if err != nil {
		return g, err
	}
	if _, err := toml.Decode(string(data), &g); err != nil {
		return g, fmt.Errorf("parse %s: %w", path, err)
	}
	g.Socket = expandPath(g.Socket)
	g.State = expandPath(g.State)
	if err := g.Validate(); err != nil {
		return g, fmt.Errorf("%s: %w", path, err)
	}
	return g, nil
}

// Validate checks pool/block invariants.
func (g Global) Validate() error {
	if g.PoolStart < 1 || g.PoolStart > 65535 {
		return fmt.Errorf("pool_start %d out of range [1, 65535]", g.PoolStart)
	}
	if g.PoolEnd <= g.PoolStart || g.PoolEnd > 65536 {
		return fmt.Errorf("pool_end %d must be > pool_start and <= 65536", g.PoolEnd)
	}
	if g.BlockSize < 1 {
		return fmt.Errorf("block_size %d must be >= 1", g.BlockSize)
	}
	if g.BlockSize > g.PoolEnd-g.PoolStart {
		return fmt.Errorf("block_size %d larger than pool [%d, %d)", g.BlockSize, g.PoolStart, g.PoolEnd)
	}
	return nil
}

// ReservedSet returns the reserved ports as a set for O(1) lookup.
func (g Global) ReservedSet() map[int]bool {
	if len(g.Reserved) == 0 {
		return nil
	}
	m := make(map[int]bool, len(g.Reserved))
	for _, p := range g.Reserved {
		m[p] = true
	}
	return m
}

// Blocks reports how many fixed-size blocks fit in the pool.
func (g Global) Blocks() int {
	return (g.PoolEnd - g.PoolStart) / g.BlockSize
}

// Service is one port-consuming service declared in a project config.
type Service struct {
	Offset  int `toml:"offset"`  // position within the instance's block (0 is Tilt's UI)
	Default int `toml:"default"` // legacy port, used when harbormaster is absent
}

// Project is the per-project configuration (harbormaster.toml at a repo root).
type Project struct {
	Name     string                    `toml:"name"`
	Services map[string]Service        `toml:"services"`
	Pins     map[string]map[string]int `toml:"pins"` // label -> service -> port
}

// ProjectPath returns the per-project config path for a repo root.
func ProjectPath(repoRoot string) string {
	return filepath.Join(repoRoot, "harbormaster.toml")
}

// LoadProject loads harbormaster.toml from repoRoot. The bool reports whether the
// file existed; a missing file is not an error.
func LoadProject(repoRoot string) (Project, bool, error) {
	path := ProjectPath(repoRoot)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Project{}, false, nil
	}
	if err != nil {
		return Project{}, false, err
	}
	var p Project
	if _, err := toml.Decode(string(data), &p); err != nil {
		return Project{}, true, fmt.Errorf("parse %s: %w", path, err)
	}
	return p, true, nil
}

// OrderedServices returns the project's service names sorted by their configured
// offset (ties broken by name). The CLI sends services in this order so the
// daemon's request-order berth assignment reproduces the configured offsets for
// the common contiguous case. See DECISIONS.md D4.
func (p Project) OrderedServices() []string {
	names := make([]string, 0, len(p.Services))
	for n := range p.Services {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		oi, oj := p.Services[names[i]].Offset, p.Services[names[j]].Offset
		if oi != oj {
			return oi < oj
		}
		return names[i] < names[j]
	})
	return names
}

// GlobalPath returns ${XDG_CONFIG_HOME:-~/.config}/harbormaster/config.toml.
func GlobalPath() string {
	return filepath.Join(configHome(), "harbormaster", "config.toml")
}

func defaultSocket() string {
	if rd := os.Getenv("XDG_RUNTIME_DIR"); rd != "" {
		return filepath.Join(rd, "harbormaster", "hm.sock")
	}
	return filepath.Join(dataHome(), "harbormaster", "hm.sock")
}

func defaultState() string {
	return filepath.Join(stateHome(), "harbormaster", "state.json")
}

func configHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	return filepath.Join(home(), ".config")
}

func stateHome() string {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return v
	}
	return filepath.Join(home(), ".local", "state")
}

func dataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	return filepath.Join(home(), ".local", "share")
}

func home() string {
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return "."
	}
	return h
}

// expandPath expands a leading ~ to the user's home directory.
func expandPath(p string) string {
	if p == "~" {
		return home()
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home(), p[2:])
	}
	return p
}

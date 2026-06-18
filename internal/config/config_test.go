package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultGlobal(t *testing.T) {
	g := DefaultGlobal()
	if g.PoolStart != 20000 || g.PoolEnd != 32000 || g.BlockSize != 20 {
		t.Fatalf("unexpected defaults: %+v", g)
	}
	if err := g.Validate(); err != nil {
		t.Fatalf("defaults should validate: %v", err)
	}
	if got := g.Blocks(); got != 600 {
		t.Errorf("Blocks() = %d, want 600", got)
	}
}

func TestLoadGlobalFrom_Missing(t *testing.T) {
	g, err := LoadGlobalFrom(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if !reflect.DeepEqual(g, DefaultGlobal()) {
		t.Errorf("missing file should yield defaults, got %+v", g)
	}
}

func TestLoadGlobalFrom_OverlaysAndKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	// Only override pool_start and reserved; block_size/pool_end keep defaults.
	content := "pool_start = 30000\nreserved = [5432, 6379]\nsocket = \"~/sock/hm.sock\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := LoadGlobalFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if g.PoolStart != 30000 {
		t.Errorf("pool_start = %d, want 30000", g.PoolStart)
	}
	if g.PoolEnd != 32000 {
		t.Errorf("pool_end = %d, want default 32000", g.PoolEnd)
	}
	if g.BlockSize != 20 {
		t.Errorf("block_size = %d, want default 20", g.BlockSize)
	}
	rs := g.ReservedSet()
	if !rs[5432] || !rs[6379] || len(rs) != 2 {
		t.Errorf("reserved set = %v, want {5432,6379}", rs)
	}
	if want := filepath.Join(home(), "sock", "hm.sock"); g.Socket != want {
		t.Errorf("socket = %q, want %q (tilde expanded)", g.Socket, want)
	}
}

func TestLoadGlobalFrom_InvalidRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("pool_end = 100\npool_start = 20000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadGlobalFrom(path); err == nil {
		t.Fatal("expected validation error for pool_end <= pool_start")
	}
}

func TestLoadProject(t *testing.T) {
	dir := t.TempDir()
	content := `name = "groundtruth"

[services]
web      = { offset = 1, default = 3000 }
api      = { offset = 2, default = 4000 }
postgres = { offset = 3, default = 5432 }

[pins.main]
web = 3000
`
	if err := os.WriteFile(ProjectPath(dir), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	p, found, err := LoadProject(dir)
	if err != nil || !found {
		t.Fatalf("LoadProject: found=%v err=%v", found, err)
	}
	if p.Name != "groundtruth" {
		t.Errorf("name = %q", p.Name)
	}
	if p.Services["api"].Offset != 2 || p.Services["api"].Default != 4000 {
		t.Errorf("api service = %+v", p.Services["api"])
	}
	if p.Pins["main"]["web"] != 3000 {
		t.Errorf("pin main.web = %d, want 3000", p.Pins["main"]["web"])
	}
	if got, want := p.OrderedServices(), []string{"web", "api", "postgres"}; !reflect.DeepEqual(got, want) {
		t.Errorf("OrderedServices() = %v, want %v", got, want)
	}
}

func TestLoadProject_Missing(t *testing.T) {
	_, found, err := LoadProject(t.TempDir())
	if err != nil || found {
		t.Fatalf("missing project: found=%v err=%v", found, err)
	}
}

func TestPaths_XDGOverrides(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/x/config")
	t.Setenv("XDG_STATE_HOME", "/x/state")
	t.Setenv("XDG_RUNTIME_DIR", "/x/run")
	if got, want := GlobalPath(), "/x/config/harbormaster/config.toml"; got != want {
		t.Errorf("GlobalPath() = %q, want %q", got, want)
	}
	if got, want := defaultState(), "/x/state/harbormaster/state.json"; got != want {
		t.Errorf("defaultState() = %q, want %q", got, want)
	}
	if got, want := defaultSocket(), "/x/run/harbormaster/hm.sock"; got != want {
		t.Errorf("defaultSocket() = %q, want %q", got, want)
	}
}

func TestDefaultSocket_NoRuntimeDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("XDG_DATA_HOME", "/x/data")
	if got, want := defaultSocket(), "/x/data/harbormaster/hm.sock"; got != want {
		t.Errorf("defaultSocket() = %q, want %q", got, want)
	}
}

package daemon

import (
	"path/filepath"
	"testing"

	"github.com/venndata-org/harbormaster/internal/config"
	"github.com/venndata-org/harbormaster/internal/ipc"
)

// TestE2E_SmokeTwoInstances is the smoke test from the build handoff: start the
// daemon on a real Unix socket, lease two fake instances of one project over the
// wire, assert their blocks/ports do NOT overlap, then release both.
func TestE2E_SmokeTwoInstances(t *testing.T) {
	cfg := config.DefaultGlobal()
	cfg.State = filepath.Join(t.TempDir(), "state.json")
	cfg.Socket = tempSocket(t)

	d, err := New(cfg, "e2e")
	if err != nil {
		t.Fatal(err)
	}
	d.Probe = freeAll // deterministic: don't touch real ports

	srv := &ipc.Server{Socket: cfg.Socket, Handler: d}
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	go srv.Serve()

	if !ipc.IsLive(cfg.Socket) {
		t.Fatal("daemon should be live after Listen+Serve")
	}

	c := &ipc.Client{Socket: cfg.Socket}
	svcs := []string{"web", "api", "postgres"}

	a, err := c.Lease("/dev/groundtruth/main", "groundtruth", "main", svcs)
	if err != nil || !a.OK {
		t.Fatalf("lease A: err=%v resp=%+v", err, a)
	}
	b, err := c.Lease("/dev/groundtruth/feat-x", "groundtruth", "feat-x", svcs)
	if err != nil || !b.OK {
		t.Fatalf("lease B: err=%v resp=%+v", err, b)
	}

	// The core guarantee: two worktrees of one project never collide.
	if a.Block == nil || b.Block == nil {
		t.Fatal("missing block in reply")
	}
	if blocksOverlap(*a.Block, *b.Block) {
		t.Fatalf("blocks overlap: %v vs %v", *a.Block, *b.Block)
	}
	if a.Tilt == b.Tilt {
		t.Fatalf("tilt ports collide: %d", a.Tilt)
	}
	for svc, pa := range a.Ports {
		if b.Ports[svc] == pa {
			t.Fatalf("service %s shares port %d across instances", svc, pa)
		}
	}

	// list reflects both leases.
	ls, err := c.List()
	if err != nil || len(ls.Instances) != 2 {
		t.Fatalf("list: err=%v resp=%+v", err, ls)
	}

	// release both; table ends empty.
	for _, inst := range []string{"/dev/groundtruth/main", "/dev/groundtruth/feat-x"} {
		r, err := c.Release(inst)
		if err != nil || !r.OK || !r.Released {
			t.Fatalf("release %s: err=%v resp=%+v", inst, err, r)
		}
	}
	ls2, err := c.List()
	if err != nil || len(ls2.Instances) != 0 {
		t.Fatalf("expected empty table after release: err=%v resp=%+v", err, ls2)
	}
}

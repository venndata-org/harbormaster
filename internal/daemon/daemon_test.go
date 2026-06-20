package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/venndata-org/harbormaster/internal/alloc"
	"github.com/venndata-org/harbormaster/internal/config"
	"github.com/venndata-org/harbormaster/internal/ipc"
	"github.com/venndata-org/harbormaster/internal/state"
)

func freeAll(int) bool { return true }

func blockPort(ports ...int) alloc.Prober {
	set := make(map[int]bool, len(ports))
	for _, p := range ports {
		set[p] = true
	}
	return func(p int) bool { return !set[p] }
}

func leaseReq(inst string, svcs ...string) ipc.Request {
	return ipc.Request{Op: "lease", Instance: inst, Project: "p", Label: "x", Services: svcs}
}

func blocksOverlap(a, b [2]int) bool { return a[0] <= b[1] && b[0] <= a[1] }

func newTestDaemon(t *testing.T, probe alloc.Prober) *Daemon {
	t.Helper()
	cfg := config.DefaultGlobal()
	cfg.State = filepath.Join(t.TempDir(), "state.json")
	d, err := New(cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	if probe != nil {
		d.Probe = probe
	}
	return d
}

func tempSocket(t *testing.T) string {
	t.Helper()
	// Keep the path short — Unix socket paths are capped (~104 bytes on macOS).
	dir, err := os.MkdirTemp("", "hmsock")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

func TestLease_TwoInstancesNonOverlapping(t *testing.T) {
	d := newTestDaemon(t, freeAll)
	a := d.Handle(leaseReq("/p/main", "web", "api"))
	b := d.Handle(leaseReq("/p/feat", "web", "api"))
	if !a.OK || !b.OK {
		t.Fatalf("lease failed: a=%+v b=%+v", a, b)
	}
	if a.Tilt != 20000 || a.Ports["web"] != 20001 || a.Ports["api"] != 20002 {
		t.Fatalf("A berths: tilt=%d ports=%v", a.Tilt, a.Ports)
	}
	if b.Tilt != 20020 || b.Ports["web"] != 20021 || b.Ports["api"] != 20022 {
		t.Fatalf("B berths: tilt=%d ports=%v", b.Tilt, b.Ports)
	}
	if *a.Block != [2]int{20000, 20019} || *b.Block != [2]int{20020, 20039} {
		t.Fatalf("blocks: %v %v", *a.Block, *b.Block)
	}
	if blocksOverlap(*a.Block, *b.Block) {
		t.Fatal("blocks overlap")
	}
}

func TestLease_StableAcrossCalls(t *testing.T) {
	d := newTestDaemon(t, freeAll)
	first := d.Handle(leaseReq("/p/main", "web"))
	// Lease a neighbour, then re-lease main — it must keep its block.
	d.Handle(leaseReq("/p/feat", "web"))
	again := d.Handle(leaseReq("/p/main", "web"))
	if again.Tilt != first.Tilt || again.Ports["web"] != first.Ports["web"] || *again.Block != *first.Block {
		t.Fatalf("not stable: first=%+v again=%+v", first, again)
	}
}

func TestLease_Persists(t *testing.T) {
	d := newTestDaemon(t, freeAll)
	d.Handle(leaseReq("/p/main", "web", "api"))
	st, err := state.Load(d.statePath)
	if err != nil {
		t.Fatal(err)
	}
	inst, ok := st.Instances["/p/main"]
	if !ok {
		t.Fatal("instance not persisted")
	}
	if inst.Berths["tilt"] != 20000 || inst.Berths["web"] != 20001 || inst.Block != [2]int{20000, 20019} {
		t.Fatalf("persisted record wrong: %+v", inst)
	}
}

func TestList_AfterLease(t *testing.T) {
	d := newTestDaemon(t, freeAll)
	d.Handle(leaseReq("/p/feat", "web"))
	d.Handle(leaseReq("/p/main", "web"))
	ls := d.list()
	if !ls.OK || len(ls.Instances) != 2 {
		t.Fatalf("list: %+v", ls)
	}
	// sorted by block base
	if ls.Instances[0].Block[0] > ls.Instances[1].Block[0] {
		t.Fatalf("not sorted by block base: %v", ls.Instances)
	}
	if ls.Instances[0].CreatedAt == "" {
		t.Error("createdAt not populated")
	}
}

func TestRelease(t *testing.T) {
	d := newTestDaemon(t, freeAll)
	d.Handle(leaseReq("/p/main", "web"))
	r := d.Handle(ipc.Request{Op: "release", Instance: "/p/main"})
	if !r.OK || !r.Released {
		t.Fatalf("release: %+v", r)
	}
	// releasing again reports released=false
	r2 := d.Handle(ipc.Request{Op: "release", Instance: "/p/main"})
	if !r2.OK || r2.Released {
		t.Fatalf("second release should be no-op: %+v", r2)
	}
}

func TestPrune(t *testing.T) {
	d := newTestDaemon(t, freeAll)
	live := t.TempDir() // a dir that exists -> survives prune
	d.Handle(leaseReq(live, "web"))
	d.Handle(leaseReq("/definitely/not/here/xyz", "web"))

	r := d.Handle(ipc.Request{Op: "prune"})
	if !r.OK || len(r.Reclaimed) != 1 || r.Reclaimed[0] != "/definitely/not/here/xyz" {
		t.Fatalf("prune: %+v", r)
	}
	ls := d.list()
	if len(ls.Instances) != 1 || ls.Instances[0].Instance != live {
		t.Fatalf("live instance should remain: %+v", ls.Instances)
	}
}

func TestDoctor(t *testing.T) {
	d := newTestDaemon(t, freeAll)
	d.Handle(leaseReq("/p/main", "web"))
	doc := d.Handle(ipc.Request{Op: "doctor"})
	if !doc.OK || doc.Leases != 1 || doc.Headroom != 599 {
		t.Fatalf("doctor: %+v", doc)
	}
	if len(doc.Squatters) != 0 {
		t.Fatalf("unexpected squatters: %+v", doc.Squatters)
	}
	// Now make web's port look occupied; doctor should flag it.
	d.Probe = blockPort(20001)
	doc2 := d.Handle(ipc.Request{Op: "doctor"})
	if len(doc2.Squatters) != 1 || doc2.Squatters[0].Port != 20001 || doc2.Squatters[0].Service != "web" {
		t.Fatalf("expected one squatter on web:20001, got %+v", doc2.Squatters)
	}
}

// TestLease_IdempotentWhileOwnPortsBound is the regression test for the
// self-conflict bug: re-leasing while the instance's own ports are in use (its
// Tilt is running) must return the same ports, not relocate them.
func TestLease_IdempotentWhileOwnPortsBound(t *testing.T) {
	d := newTestDaemon(t, freeAll)
	first := d.Handle(leaseReq("/p/main", "web", "api"))
	if !first.OK {
		t.Fatalf("first lease: %+v", first)
	}
	// Simulate the running Tilt/services holding those ports.
	d.Probe = blockPort(first.Tilt, first.Ports["web"], first.Ports["api"])

	again := d.Handle(leaseReq("/p/main", "web", "api"))
	if again.Tilt != first.Tilt || again.Ports["web"] != first.Ports["web"] || again.Ports["api"] != first.Ports["api"] {
		t.Fatalf("re-lease while running must be stable:\n first=%+v\n again=%+v", first, again)
	}
	if len(again.Warnings) != 0 {
		t.Fatalf("no relocation expected, got warnings %v", again.Warnings)
	}
}

func TestGet_ReadOnly(t *testing.T) {
	d := newTestDaemon(t, freeAll)
	leased := d.Handle(leaseReq("/p/main", "web"))
	// Even with its ports "in use", get returns the stored ports and never relocates.
	d.Probe = blockPort(leased.Tilt, leased.Ports["web"])

	got := d.Handle(ipc.Request{Op: "get", Instance: "/p/main"})
	if !got.OK || !got.Found {
		t.Fatalf("get should find the lease: %+v", got)
	}
	if got.Tilt != leased.Tilt || got.Ports["web"] != leased.Ports["web"] {
		t.Fatalf("get should return stored ports unchanged: got %+v, leased %+v", got, leased)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("get must not probe/relocate, got warnings %v", got.Warnings)
	}
	if got.Block == nil || *got.Block != [2]int{20000, 20019} {
		t.Fatalf("get block wrong: %v", got.Block)
	}
}

func TestGet_NotFound(t *testing.T) {
	d := newTestDaemon(t, freeAll)
	got := d.Handle(ipc.Request{Op: "get", Instance: "/never/leased"})
	if !got.OK || got.Found {
		t.Fatalf("get on an unleased instance should be ok with found=false: %+v", got)
	}
}

func TestGet_MissingInstance(t *testing.T) {
	d := newTestDaemon(t, freeAll)
	if r := d.Handle(ipc.Request{Op: "get"}); r.OK {
		t.Fatalf("get without instance should error: %+v", r)
	}
}

func TestUnknownOp(t *testing.T) {
	d := newTestDaemon(t, freeAll)
	if r := d.Handle(ipc.Request{Op: "bogus"}); r.OK || r.Error == "" {
		t.Fatalf("expected error for unknown op: %+v", r)
	}
}

func TestLease_MissingInstance(t *testing.T) {
	d := newTestDaemon(t, freeAll)
	if r := d.Handle(ipc.Request{Op: "lease", Services: []string{"web"}}); r.OK {
		t.Fatalf("expected error for missing instance: %+v", r)
	}
}

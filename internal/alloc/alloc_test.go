package alloc

import (
	"reflect"
	"testing"
)

func freeAll(int) bool { return true }

// blockedPorts returns a Prober that reports the given ports as occupied.
func blockedPorts(ports ...int) Prober {
	set := make(map[int]bool, len(ports))
	for _, p := range ports {
		set[p] = true
	}
	return func(p int) bool { return !set[p] }
}

func defaultPool() Pool {
	return Pool{Start: 20000, End: 32000, BlockSize: 20}
}

// TestAllocate_WorkedExample reproduces SPEC §4 exactly: two worktrees of one
// project land in adjacent, non-overlapping blocks with deterministic berths.
func TestAllocate_WorkedExample(t *testing.T) {
	pool := defaultPool()
	svcs := []string{"web", "api", "postgres", "grafana"}
	existing := map[string]Block{}

	a, err := Allocate(pool, existing, Request{Instance: "/p/main", Services: svcs}, freeAll)
	if err != nil {
		t.Fatal(err)
	}
	if a.Tilt != 20000 || a.Ports["web"] != 20001 || a.Ports["api"] != 20002 ||
		a.Ports["postgres"] != 20003 || a.Ports["grafana"] != 20004 {
		t.Fatalf("instance A berths wrong: %+v", a)
	}
	if a.Block.Range() != [2]int{20000, 20019} {
		t.Fatalf("A block = %v, want [20000 20019]", a.Block.Range())
	}

	existing["/p/main"] = a.Block
	b, err := Allocate(pool, existing, Request{Instance: "/p/feat-x", Services: svcs}, freeAll)
	if err != nil {
		t.Fatal(err)
	}
	if b.Tilt != 20020 || b.Ports["web"] != 20021 || b.Ports["grafana"] != 20024 {
		t.Fatalf("instance B berths wrong: %+v", b)
	}
	if a.Block.Overlaps(b.Block) {
		t.Fatalf("blocks overlap: %v vs %v", a.Block.Range(), b.Block.Range())
	}
}

// TestAllocate_NonOverlapping allocates many instances and asserts every block
// and every port is unique — the core guarantee.
func TestAllocate_NonOverlapping(t *testing.T) {
	pool := defaultPool()
	svcs := []string{"web", "api"}
	existing := map[string]Block{}
	seenPort := map[int]bool{}
	var blocks []Block

	for i := 0; i < 25; i++ {
		inst := "/p/wt" + string(rune('a'+i))
		r, err := Allocate(pool, existing, Request{Instance: inst, Services: svcs}, freeAll)
		if err != nil {
			t.Fatalf("alloc %d: %v", i, err)
		}
		for _, b := range blocks {
			if b.Overlaps(r.Block) {
				t.Fatalf("block %v overlaps existing %v", r.Block.Range(), b.Range())
			}
		}
		blocks = append(blocks, r.Block)
		for _, p := range append([]int{r.Tilt}, r.Ports["web"], r.Ports["api"]) {
			if seenPort[p] {
				t.Fatalf("port %d handed out twice", p)
			}
			seenPort[p] = true
		}
		existing[inst] = r.Block
	}
}

// TestAllocate_Stable: re-leasing an instance returns its original block/ports.
func TestAllocate_Stable(t *testing.T) {
	pool := defaultPool()
	svcs := []string{"web", "api"}
	existing := map[string]Block{}

	first, _ := Allocate(pool, existing, Request{Instance: "/p/main", Services: svcs}, freeAll)
	existing["/p/main"] = first.Block
	// Add a neighbour so a naive "lowest free" would move it.
	existing["/p/other"] = Block{Base: 20020, Size: 20}

	again, err := Allocate(pool, existing, Request{Instance: "/p/main", Services: svcs}, freeAll)
	if err != nil {
		t.Fatal(err)
	}
	if again.Block != first.Block || !reflect.DeepEqual(again.Ports, first.Ports) || again.Tilt != first.Tilt {
		t.Fatalf("not stable: first %+v, again %+v", first, again)
	}
}

// TestAllocate_HoleFilling: a freed middle block is reused before extending.
func TestAllocate_HoleFilling(t *testing.T) {
	pool := defaultPool()
	existing := map[string]Block{
		"/p/a": {Base: 20000, Size: 20},
		"/p/c": {Base: 20040, Size: 20},
	}
	r, err := Allocate(pool, existing, Request{Instance: "/p/b", Services: []string{"web"}}, freeAll)
	if err != nil {
		t.Fatal(err)
	}
	if r.Block.Base != 20020 {
		t.Fatalf("expected hole at 20020, got base %d", r.Block.Base)
	}
}

// TestAllocate_ReuseHonorsBase: stability wins over lowest-free for known instances.
func TestAllocate_ReuseHonorsBase(t *testing.T) {
	pool := defaultPool()
	existing := map[string]Block{"/p/main": {Base: 20060, Size: 20}}
	r, err := Allocate(pool, existing, Request{Instance: "/p/main", Services: []string{"web"}}, freeAll)
	if err != nil {
		t.Fatal(err)
	}
	if r.Block.Base != 20060 || r.Tilt != 20060 || r.Ports["web"] != 20061 {
		t.Fatalf("reuse should honor base 20060: %+v", r)
	}
}

// TestAllocate_ReservedSkip: a reserved nominal port forces berth relocation.
func TestAllocate_ReservedSkip(t *testing.T) {
	pool := defaultPool()
	pool.Reserved = map[int]bool{20001: true}
	r, err := Allocate(pool, map[string]Block{}, Request{Instance: "/p/main", Services: []string{"web"}}, freeAll)
	if err != nil {
		t.Fatal(err)
	}
	if r.Tilt != 20000 {
		t.Errorf("tilt = %d, want 20000", r.Tilt)
	}
	if r.Ports["web"] != 20002 {
		t.Errorf("web = %d, want relocated 20002 (20001 reserved)", r.Ports["web"])
	}
	if len(r.Warnings) == 0 {
		t.Error("expected a relocation warning")
	}
}

// TestAllocate_SquatterRelocation: a port held by an external process relocates.
func TestAllocate_SquatterRelocation(t *testing.T) {
	pool := defaultPool()
	probe := blockedPorts(20002) // api's nominal port is squatted
	r, err := Allocate(pool, map[string]Block{}, Request{Instance: "/p/main", Services: []string{"web", "api"}}, probe)
	if err != nil {
		t.Fatal(err)
	}
	if r.Ports["web"] != 20001 {
		t.Errorf("web = %d, want 20001", r.Ports["web"])
	}
	if r.Ports["api"] != 20003 {
		t.Errorf("api = %d, want relocated 20003 (20002 squatted)", r.Ports["api"])
	}
	if len(r.Warnings) == 0 {
		t.Error("expected a relocation warning")
	}
}

// TestAllocate_SkipsFullyContestedBlock: a new instance steps over a block whose
// berths can't be satisfied and lands in the next one.
func TestAllocate_SkipsFullyContestedBlock(t *testing.T) {
	pool := Pool{Start: 20000, End: 20040, BlockSize: 20}
	// Block 20000..20019 fully occupied -> no usable berth at all.
	blocked := make([]int, 0, 20)
	for p := 20000; p < 20020; p++ {
		blocked = append(blocked, p)
	}
	r, err := Allocate(pool, map[string]Block{}, Request{Instance: "/p/main", Services: []string{"web"}}, blockedPorts(blocked...))
	if err != nil {
		t.Fatal(err)
	}
	if r.Block.Base != 20020 {
		t.Fatalf("expected to skip contested block, got base %d", r.Block.Base)
	}
}

// TestAllocate_PoolExhausted: no free block -> ErrPoolExhausted.
func TestAllocate_PoolExhausted(t *testing.T) {
	pool := Pool{Start: 20000, End: 20040, BlockSize: 20} // exactly 2 blocks
	existing := map[string]Block{
		"/p/a": {Base: 20000, Size: 20},
		"/p/b": {Base: 20020, Size: 20},
	}
	_, err := Allocate(pool, existing, Request{Instance: "/p/c", Services: []string{"web"}}, freeAll)
	if err != ErrPoolExhausted {
		t.Fatalf("err = %v, want ErrPoolExhausted", err)
	}
}

// TestAllocate_BlockTooSmall: more berths than the block can hold.
func TestAllocate_BlockTooSmall(t *testing.T) {
	pool := Pool{Start: 20000, End: 32000, BlockSize: 2} // room for tilt + 1 service
	_, err := Allocate(pool, map[string]Block{}, Request{Instance: "/p/main", Services: []string{"web", "api"}}, freeAll)
	if err == nil || err == ErrPoolExhausted {
		t.Fatalf("expected block-too-small error, got %v", err)
	}
}

// TestAllocate_Deterministic: identical inputs (even under contention) -> identical
// results, warnings included.
func TestAllocate_Deterministic(t *testing.T) {
	pool := defaultPool()
	pool.Reserved = map[int]bool{20002: true}
	probe := blockedPorts(20005)
	req := Request{Instance: "/p/main", Services: []string{"web", "api", "db", "cache"}}

	r1, _ := Allocate(pool, map[string]Block{}, req, probe)
	r2, _ := Allocate(pool, map[string]Block{}, req, probe)
	if !reflect.DeepEqual(r1, r2) {
		t.Fatalf("non-deterministic:\n r1=%+v\n r2=%+v", r1, r2)
	}
}

func TestBlock_Helpers(t *testing.T) {
	b := Block{Base: 20020, Size: 20}
	if b.Hi() != 20039 {
		t.Errorf("Hi() = %d, want 20039", b.Hi())
	}
	if b.Range() != [2]int{20020, 20039} {
		t.Errorf("Range() = %v", b.Range())
	}
	if !b.Overlaps(Block{Base: 20039, Size: 5}) {
		t.Error("should overlap at boundary")
	}
	if b.Overlaps(Block{Base: 20040, Size: 5}) {
		t.Error("should not overlap just past boundary")
	}
}

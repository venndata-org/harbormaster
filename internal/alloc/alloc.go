// Package alloc is the deterministic block allocator — the heart of harbormaster.
//
// Each instance (checkout) gets a contiguous block of ports from a machine-wide
// pool. The block base is stable once assigned; new instances take the lowest
// free block. Within a block, offset 0 is the Tilt UI port and requested services
// take offsets 1, 2, 3, … in order. Before a port is handed out it is bind-probed
// and checked against the reserved list, relocating within the block when a
// nominal port is taken. See SPEC.md §4 and DECISIONS.md D4/D5.
package alloc

import (
	"errors"
	"fmt"
	"net"
	"strconv"
)

// ErrPoolExhausted is returned when no free block can host a new instance.
var ErrPoolExhausted = errors.New("port pool exhausted: no free block available")

// Block is a contiguous port range [Base, Base+Size) reserved for one instance.
type Block struct {
	Base int
	Size int
}

// Hi is the inclusive top port of the block.
func (b Block) Hi() int { return b.Base + b.Size - 1 }

// Range returns the inclusive [lo, hi] pair persisted in state.json (SPEC §9).
func (b Block) Range() [2]int { return [2]int{b.Base, b.Hi()} }

// Overlaps reports whether two blocks share any port.
func (b Block) Overlaps(o Block) bool {
	return b.Base <= o.Hi() && o.Base <= b.Hi()
}

// Pool is the machine-wide port range, divided into fixed-size blocks.
type Pool struct {
	Start     int          // inclusive
	End       int          // exclusive (half-open): ports run [Start, End)
	BlockSize int          // ports per block
	Reserved  map[int]bool // never handed out
}

// Request asks for a lease for one instance and its ordered services.
type Request struct {
	Instance string   // absolute checkout path; the allocation key
	Services []string // ordered service names; the implicit Tilt port is separate
}

// Prober reports whether a port is free (bindable) on the loopback interface.
// Production uses TCPProbe; tests inject a deterministic fake.
type Prober func(port int) bool

// Result is one instance's resolved lease.
type Result struct {
	Block    Block
	Tilt     int            // the Tilt UI port (0 if unassignable)
	Ports    map[string]int // service -> port (0/absent if unassignable)
	Warnings []string       // non-fatal notes, e.g. a relocated berth
}

// Complete reports whether every requested berth got a real port.
func (r Result) Complete(services []string) bool {
	if r.Tilt <= 0 {
		return false
	}
	for _, s := range services {
		if r.Ports[s] <= 0 {
			return false
		}
	}
	return true
}

// Allocate resolves a lease for req against the pool and the currently-leased
// blocks (instance path -> block). It is deterministic: the same existing leases
// + request + prober yield the same result. existing must not include leases for
// other instances that overlap the pool inconsistently; the daemon owns that
// invariant. If probe is nil, TCPProbe is used.
func Allocate(pool Pool, existing map[string]Block, req Request, probe Prober) (Result, error) {
	if probe == nil {
		probe = TCPProbe
	}
	need := 1 + len(req.Services)
	if need > pool.BlockSize {
		return Result{}, fmt.Errorf("block_size %d too small for %d berths (tilt + %d services)",
			pool.BlockSize, need, len(req.Services))
	}

	// Reuse a stable block if this instance already holds one.
	if b, ok := existing[req.Instance]; ok {
		b.Size = pool.BlockSize
		return assignWithin(b, req.Services, probe, pool.Reserved), nil
	}

	// New instance: take the lowest free block that can host the berths.
	occupied := occupiedBases(existing, req.Instance)
	for base := pool.Start; base+pool.BlockSize <= pool.End; base += pool.BlockSize {
		if occupied[base] {
			continue
		}
		res := assignWithin(Block{Base: base, Size: pool.BlockSize}, req.Services, probe, pool.Reserved)
		if res.Complete(req.Services) {
			return res, nil
		}
	}
	return Result{}, ErrPoolExhausted
}

// assignWithin places tilt (offset 0) and each service (offsets 1..N) inside the
// block, relocating to another free offset when a nominal port is reserved or in
// use, and recording a warning when it does.
func assignWithin(block Block, services []string, probe Prober, reserved map[int]bool) Result {
	res := Result{Block: block, Ports: make(map[string]int, len(services))}
	used := make(map[int]bool, block.Size)

	usable := func(off int) bool {
		if off < 0 || off >= block.Size || used[off] {
			return false
		}
		port := block.Base + off
		if reserved[port] {
			return false
		}
		return probe(port)
	}

	choose := func(nominal int, label string) int {
		if usable(nominal) {
			used[nominal] = true
			return block.Base + nominal
		}
		for off := 0; off < block.Size; off++ {
			if usable(off) {
				used[off] = true
				res.Warnings = append(res.Warnings, fmt.Sprintf(
					"%s: nominal port %d unavailable (reserved or in use), relocated to %d",
					label, block.Base+nominal, block.Base+off))
				return block.Base + off
			}
		}
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"%s: no free port in block [%d, %d]", label, block.Base, block.Hi()))
		return 0
	}

	res.Tilt = choose(0, "tilt")
	for i, svc := range services {
		res.Ports[svc] = choose(1+i, svc)
	}
	return res
}

func occupiedBases(existing map[string]Block, self string) map[int]bool {
	m := make(map[int]bool, len(existing))
	for inst, b := range existing {
		if inst == self {
			continue
		}
		m[b.Base] = true
	}
	return m
}

// TCPProbe reports whether port is free by binding 127.0.0.1:port and releasing
// it immediately. Binding loopback catches both 127.0.0.1 and 0.0.0.0 holders.
func TCPProbe(port int) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

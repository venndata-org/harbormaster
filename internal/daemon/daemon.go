// Package daemon is the harbormaster lease engine. It implements ipc.Handler by
// wiring the allocator, the persisted state table, and the config under one
// mutex. It lives in its own package (not cmd/) so both harbormasterd and the
// CLI's `hm daemon` can run it. See SPEC.md §3/§8/§9.
package daemon

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/venndata-org/harbormaster/internal/alloc"
	"github.com/venndata-org/harbormaster/internal/config"
	"github.com/venndata-org/harbormaster/internal/ipc"
	"github.com/venndata-org/harbormaster/internal/state"
)

// ErrAlreadyRunning is returned by Run when a daemon already owns the socket.
var ErrAlreadyRunning = errors.New("daemon already running")

// Daemon holds the lease table and serves ipc requests. Safe for concurrent use.
type Daemon struct {
	cfg     config.Global
	pool    alloc.Pool
	version string

	// Probe decides whether a candidate port is free. Defaults to alloc.TCPProbe;
	// tests inject a deterministic fake.
	Probe alloc.Prober

	mu        sync.Mutex
	st        *state.State
	statePath string
}

// New builds a Daemon, loading any existing state from cfg.State.
func New(cfg config.Global, version string) (*Daemon, error) {
	st, err := state.Load(cfg.State)
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	return &Daemon{
		cfg:       cfg,
		pool:      alloc.Pool{Start: cfg.PoolStart, End: cfg.PoolEnd, BlockSize: cfg.BlockSize, Reserved: cfg.ReservedSet()},
		version:   version,
		Probe:     alloc.TCPProbe,
		st:        st,
		statePath: cfg.State,
	}, nil
}

// Handle dispatches one request. It is the ipc.Handler entrypoint.
func (d *Daemon) Handle(req ipc.Request) ipc.Response {
	switch req.Op {
	case "ping":
		return ipc.Response{OK: true, Version: d.version}
	case "lease":
		return d.lease(req)
	case "list":
		return d.list()
	case "release":
		return d.release(req)
	case "prune":
		return d.prune()
	case "doctor":
		return d.doctor()
	default:
		return ipc.Err("unknown op: %q", req.Op)
	}
}

func (d *Daemon) lease(req ipc.Request) ipc.Response {
	if req.Instance == "" {
		return ipc.Err("lease: missing instance")
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	res, err := alloc.Allocate(d.pool, d.blocks(), alloc.Request{Instance: req.Instance, Services: req.Services}, d.Probe)
	if err != nil {
		return ipc.Err("%v", err)
	}

	now := time.Now().UTC()
	inst, ok := d.st.Instances[req.Instance]
	if !ok {
		inst = &state.Instance{CreatedAt: now}
		d.st.Instances[req.Instance] = inst
	}
	inst.Project = req.Project
	inst.Label = req.Label
	inst.Block = res.Block.Range()
	berths := map[string]int{"tilt": res.Tilt}
	for svc, port := range res.Ports {
		berths[svc] = port
	}
	inst.Berths = berths
	inst.LastSeenAt = now

	if err := state.Save(d.statePath, d.st); err != nil {
		return ipc.Err("persist: %v", err)
	}

	blk := res.Block.Range()
	return ipc.Response{OK: true, Tilt: res.Tilt, Ports: res.Ports, Block: &blk, Warnings: res.Warnings}
}

func (d *Daemon) list() ipc.Response {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]ipc.InstanceInfo, 0, len(d.st.Instances))
	for path, inst := range d.st.Instances {
		out = append(out, ipc.InstanceInfo{
			Instance:   path,
			Project:    inst.Project,
			Label:      inst.Label,
			Block:      inst.Block,
			Berths:     inst.Berths,
			CreatedAt:  iso(inst.CreatedAt),
			LastSeenAt: iso(inst.LastSeenAt),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Block[0] < out[j].Block[0] })
	return ipc.Response{OK: true, Instances: out}
}

func (d *Daemon) release(req ipc.Request) ipc.Response {
	if req.Instance == "" {
		return ipc.Err("release: missing instance")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.st.Instances[req.Instance]
	if ok {
		delete(d.st.Instances, req.Instance)
		if err := state.Save(d.statePath, d.st); err != nil {
			return ipc.Err("persist: %v", err)
		}
	}
	return ipc.Response{OK: true, Released: ok}
}

func (d *Daemon) prune() ipc.Response {
	d.mu.Lock()
	defer d.mu.Unlock()
	var reclaimed []string
	for path := range d.st.Instances {
		if !dirExists(path) {
			reclaimed = append(reclaimed, path)
		}
	}
	for _, path := range reclaimed {
		delete(d.st.Instances, path)
	}
	if len(reclaimed) > 0 {
		if err := state.Save(d.statePath, d.st); err != nil {
			return ipc.Err("persist: %v", err)
		}
	}
	sort.Strings(reclaimed)
	return ipc.Response{OK: true, Reclaimed: reclaimed}
}

func (d *Daemon) doctor() ipc.Response {
	d.mu.Lock()
	defer d.mu.Unlock()
	var squatters []ipc.Squatter
	for path, inst := range d.st.Instances {
		for svc, port := range inst.Berths {
			if !d.Probe(port) {
				squatters = append(squatters, ipc.Squatter{Instance: path, Service: svc, Port: port})
			}
		}
	}
	sort.Slice(squatters, func(i, j int) bool { return squatters[i].Port < squatters[j].Port })
	return ipc.Response{OK: true, Leases: len(d.st.Instances), Headroom: d.headroom(), Squatters: squatters}
}

// blocks projects the state table into the allocator's view.
func (d *Daemon) blocks() map[string]alloc.Block {
	m := make(map[string]alloc.Block, len(d.st.Instances))
	for path, inst := range d.st.Instances {
		m[path] = alloc.Block{Base: inst.Block[0], Size: inst.Block[1] - inst.Block[0] + 1}
	}
	return m
}

// headroom is the number of free blocks left in the pool.
func (d *Daemon) headroom() int {
	total := (d.pool.End - d.pool.Start) / d.pool.BlockSize
	bases := make(map[int]bool, len(d.st.Instances))
	for _, inst := range d.st.Instances {
		bases[inst.Block[0]] = true
	}
	return total - len(bases)
}

// Run serves the daemon until a signal arrives, owning the socket lifecycle. It
// returns ErrAlreadyRunning if another daemon already answers on the socket.
func Run(cfg config.Global, version string) error {
	if ipc.IsLive(cfg.Socket) {
		return ErrAlreadyRunning
	}
	d, err := New(cfg, version)
	if err != nil {
		return err
	}
	srv := &ipc.Server{Socket: cfg.Socket, Handler: d}
	if err := srv.Listen(); err != nil {
		return err
	}
	defer srv.Close()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		srv.Close()
	}()

	log.Printf("harbormasterd %s listening on %s (pool [%d,%d) block %d)",
		version, cfg.Socket, cfg.PoolStart, cfg.PoolEnd, cfg.BlockSize)
	return srv.Serve()
}

func iso(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

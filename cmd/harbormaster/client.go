package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/venndata-org/harbormaster/internal/config"
	"github.com/venndata-org/harbormaster/internal/daemon"
	"github.com/venndata-org/harbormaster/internal/gitident"
	"github.com/venndata-org/harbormaster/internal/ipc"
)

// daemonClient returns a client to the daemon, auto-starting it if the socket is
// dead and restarting it if a stale (different-version) daemon is answering. The
// version check stops the new CLI from talking to an old daemon that doesn't know
// newer ops. See DECISIONS.md D11.
func daemonClient() (*ipc.Client, config.Global, error) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return nil, cfg, fmt.Errorf("config: %w", err)
	}
	c := &ipc.Client{Socket: cfg.Socket}
	if !ipc.IsLive(cfg.Socket) {
		if err := startDaemon(cfg); err != nil {
			return nil, cfg, err
		}
		return c, cfg, nil
	}
	// A daemon is answering — make sure it's our version, not a stale one.
	if ping, err := c.Ping(); err == nil && ping.Version != "" && ping.Version != version {
		if err := restartDaemon(cfg, c); err != nil {
			return nil, cfg, fmt.Errorf("restart stale daemon (%s -> %s): %w", ping.Version, version, err)
		}
	}
	return c, cfg, nil
}

// restartDaemon stops a stale daemon (graceful shutdown op, then a pidfile signal
// as fallback) and starts a fresh one from this binary.
func restartDaemon(cfg config.Global, c *ipc.Client) error {
	_, _ = c.Shutdown() // graceful; unknown to pre-D11 daemons
	if !waitDead(cfg.Socket, 2*time.Second) {
		if pid := readDaemonPID(cfg); pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGTERM)
			if !waitDead(cfg.Socket, 2*time.Second) {
				_ = syscall.Kill(pid, syscall.SIGKILL)
				waitDead(cfg.Socket, 2*time.Second)
			}
		}
	}
	if ipc.IsLive(cfg.Socket) {
		return errors.New("could not stop the running daemon; kill it manually (lsof " + cfg.Socket + ")")
	}
	_ = os.Remove(cfg.Socket)
	return startDaemon(cfg)
}

// waitDead polls until no daemon answers on the socket, or the timeout elapses.
func waitDead(socket string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if !ipc.IsLive(socket) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// readDaemonPID reads the daemon's pidfile, returning 0 if absent/unreadable.
func readDaemonPID(cfg config.Global) int {
	data, err := os.ReadFile(daemon.PidPath(cfg))
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}

// startDaemon re-execs this binary as `hm daemon`, detached into its own session,
// then waits for the socket to come up. Re-execing self avoids needing a separate
// harbormasterd binary on PATH. See DECISIONS.md D7.
func startDaemon(cfg config.Global) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate self: %w", err)
	}
	logPath := filepath.Join(filepath.Dir(cfg.State), "harbormasterd.log")
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	logf, _ := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)

	cmd := exec.Command(exe, "daemon")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // survive the CLI exit
	cmd.Stdin = nil
	if logf != nil {
		cmd.Stdout = logf
		cmd.Stderr = logf
	}
	if err := cmd.Start(); err != nil {
		if logf != nil {
			_ = logf.Close()
		}
		return fmt.Errorf("start daemon: %w", err)
	}
	_ = cmd.Process.Release()
	if logf != nil {
		_ = logf.Close()
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ipc.IsLive(cfg.Socket) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not become ready at %s (see %s)", cfg.Socket, logPath)
}

// identity resolves git identity for $PWD and overlays the project config name.
func identity() (gitident.Identity, config.Project, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return gitident.Identity{}, config.Project{}, err
	}
	id, err := gitident.Resolve(cwd)
	if err != nil {
		return id, config.Project{}, err
	}
	proj, _, err := config.LoadProject(id.RepoRoot)
	if err != nil {
		return id, proj, err
	}
	if proj.Name != "" {
		id.Project = proj.Name
	}
	return id, proj, nil
}

// leaseCurrent resolves $PWD's identity and leases its ports.
func leaseCurrent() (ipc.Response, gitident.Identity, config.Project, error) {
	id, proj, err := identity()
	if err != nil {
		return ipc.Response{}, id, proj, err
	}
	c, _, err := daemonClient()
	if err != nil {
		return ipc.Response{}, id, proj, err
	}
	resp, err := c.Lease(id.Instance, id.Project, id.Label, proj.OrderedServices())
	if err != nil {
		return ipc.Response{}, id, proj, err
	}
	if !resp.OK {
		return resp, id, proj, errors.New(resp.Error)
	}
	return resp, id, proj, nil
}

// portsCurrent resolves $PWD's ports read-only when a lease already exists,
// allocating one only on first use. This keeps `hm ports` from re-probing and
// relocating an instance's own running ports. See DECISIONS.md D10.
func portsCurrent() (ipc.Response, gitident.Identity, config.Project, error) {
	id, proj, err := identity()
	if err != nil {
		return ipc.Response{}, id, proj, err
	}
	c, _, err := daemonClient()
	if err != nil {
		return ipc.Response{}, id, proj, err
	}
	resp, err := c.Get(id.Instance)
	if err != nil {
		return ipc.Response{}, id, proj, err
	}
	if !resp.OK {
		return resp, id, proj, errors.New(resp.Error)
	}
	if resp.Found {
		return resp, id, proj, nil // existing lease — pure read, no mutation
	}
	// No lease yet: create one on first use.
	resp, err = c.Lease(id.Instance, id.Project, id.Label, proj.OrderedServices())
	if err != nil {
		return ipc.Response{}, id, proj, err
	}
	if !resp.OK {
		return resp, id, proj, errors.New(resp.Error)
	}
	return resp, id, proj, nil
}

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/venndata-org/harbormaster/internal/config"
	"github.com/venndata-org/harbormaster/internal/gitident"
	"github.com/venndata-org/harbormaster/internal/ipc"
)

// daemonClient returns a client to the daemon, auto-starting it if the socket is
// dead.
func daemonClient() (*ipc.Client, config.Global, error) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return nil, cfg, fmt.Errorf("config: %w", err)
	}
	if !ipc.IsLive(cfg.Socket) {
		if err := startDaemon(cfg); err != nil {
			return nil, cfg, err
		}
	}
	return &ipc.Client{Socket: cfg.Socket}, cfg, nil
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

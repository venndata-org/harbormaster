package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/venndata-org/harbormaster/internal/config"
	"github.com/venndata-org/harbormaster/internal/daemon"
	"github.com/venndata-org/harbormaster/internal/gitident"
	"github.com/venndata-org/harbormaster/internal/ipc"
)

// cmdInit writes a starter harbormaster.toml at the repo root.
func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	force := fs.Bool("force", false, "overwrite an existing harbormaster.toml")
	name := fs.String("name", "", "project name (default: derived from git)")
	var svcs serviceList
	fs.Var(&svcs, "service", "service as name:default (repeatable), e.g. -service web:3000")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	id, err := gitident.Resolve(cwd)
	if err != nil {
		return err
	}
	path := config.ProjectPath(id.RepoRoot)
	if _, err := os.Stat(path); err == nil && !*force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", path)
	}
	projName := *name
	if projName == "" {
		projName = id.Project
	}
	if err := os.WriteFile(path, []byte(renderProjectTOML(projName, svcs)), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (project %q, %d service(s))\n", path, projName, len(svcs))
	fmt.Println("commit this file so teammates share the service topology; run `hm up` to launch.")
	return nil
}

// cmdUp leases this checkout's ports and execs `tilt up --port <tilt>`.
func cmdUp(args []string) error {
	resp, _, _, err := leaseCurrent()
	if err != nil {
		return err
	}
	printWarnings(resp)
	tiltArgs := append([]string{"up", "--port", strconv.Itoa(resp.Tilt)}, tiltPassthrough(args)...)
	return runTilt(hmEnvLines(resp), tiltArgs)
}

// cmdDown runs `tilt down` with this checkout's HM_* env. The lease is kept (its
// block stays stable); use `hm release` to free it. See DECISIONS.md D9.
func cmdDown(args []string) error {
	resp, _, _, err := leaseCurrent()
	if err != nil {
		return err
	}
	return runTilt(hmEnvLines(resp), append([]string{"down"}, tiltPassthrough(args)...))
}

// cmdPorts prints this checkout's berths in the requested format.
func cmdPorts(args []string) error {
	fs := flag.NewFlagSet("ports", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print machine-readable JSON")
	asEnv := fs.Bool("env", false, "print shell export lines (HM_TILT_PORT/HM_PORT_*)")
	write := fs.Bool("write", false, "write .harbormaster.env at the repo root")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	resp, id, _, err := leaseCurrent()
	if err != nil {
		return err
	}
	printWarnings(resp)
	switch {
	case *asJSON:
		return printPortsJSON(os.Stdout, id, resp)
	case *asEnv:
		fmt.Println(strings.Join(hmEnvLines(resp), "\n"))
	case *write:
		path := filepath.Join(id.RepoRoot, ".harbormaster.env")
		body := strings.Join(hmEnvLines(resp), "\n") + "\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", path)
	default:
		printPortsHuman(os.Stdout, id, resp)
	}
	return nil
}

// cmdLs prints every leased instance.
func cmdLs(args []string) error {
	c, _, err := daemonClient()
	if err != nil {
		return err
	}
	resp, err := c.List()
	if err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	printList(os.Stdout, resp.Instances)
	return nil
}

// cmdRelease frees a lease (default: this checkout).
func cmdRelease(args []string) error {
	fs := flag.NewFlagSet("release", flag.ContinueOnError)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	instance, err := releaseTarget(fs.Args())
	if err != nil {
		return err
	}
	c, _, err := daemonClient()
	if err != nil {
		return err
	}
	resp, err := c.Release(instance)
	if err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	if resp.Released {
		fmt.Printf("released %s\n", instance)
	} else {
		fmt.Printf("no lease for %s\n", instance)
	}
	return nil
}

// releaseTarget resolves the instance path to release from optional args.
func releaseTarget(args []string) (string, error) {
	if len(args) > 0 {
		target := args[0]
		if id, err := gitident.Resolve(target); err == nil {
			return id.Instance, nil
		}
		// Not a live git worktree (e.g. already deleted): release by absolute path.
		return filepath.Abs(target)
	}
	id, _, err := identity()
	if err != nil {
		return "", err
	}
	return id.Instance, nil
}

// cmdPrune reclaims leases whose worktree directory is gone.
func cmdPrune(args []string) error {
	c, _, err := daemonClient()
	if err != nil {
		return err
	}
	resp, err := c.Prune()
	if err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	if len(resp.Reclaimed) == 0 {
		fmt.Println("nothing to prune")
		return nil
	}
	fmt.Printf("reclaimed %d lease(s):\n", len(resp.Reclaimed))
	for _, p := range resp.Reclaimed {
		fmt.Printf("  %s\n", p)
	}
	return nil
}

// cmdDoctor reports daemon and pool health.
func cmdDoctor(args []string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	fmt.Printf("socket:    %s\n", cfg.Socket)
	fmt.Printf("state:     %s\n", cfg.State)
	fmt.Printf("pool:      [%d, %d)  block %d  (%d blocks)\n",
		cfg.PoolStart, cfg.PoolEnd, cfg.BlockSize, (cfg.PoolEnd-cfg.PoolStart)/cfg.BlockSize)

	c, _, err := daemonClient()
	if err != nil {
		fmt.Printf("daemon:    NOT running (%v)\n", err)
		return nil
	}
	if ping, err := c.Ping(); err == nil {
		fmt.Printf("daemon:    running (version %s)\n", ping.Version)
	}
	doc, err := c.Doctor()
	if err != nil {
		return err
	}
	if !doc.OK {
		return errors.New(doc.Error)
	}
	fmt.Printf("leases:    %d\n", doc.Leases)
	fmt.Printf("headroom:  %d free blocks\n", doc.Headroom)
	if len(doc.Squatters) == 0 {
		fmt.Println("squatters: none")
	} else {
		fmt.Printf("squatters: %d leased port(s) in use (expected while Tilt runs):\n", len(doc.Squatters))
		for _, s := range doc.Squatters {
			fmt.Printf("  %s  %s:%d\n", s.Instance, s.Service, s.Port)
		}
	}
	return nil
}

// cmdDaemon runs the daemon in the foreground (also the auto-start re-exec target).
func cmdDaemon(args []string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	if err := daemon.Run(cfg, version); err != nil {
		if errors.Is(err, daemon.ErrAlreadyRunning) {
			fmt.Printf("daemon already running at %s\n", cfg.Socket)
			return nil
		}
		return err
	}
	return nil
}

// --- helpers ---

// parseFlags parses args and treats -h/--help as a clean (non-error) exit.
func parseFlags(fs *flag.FlagSet, args []string) error {
	err := fs.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	return err
}

// tiltPassthrough strips a leading "--" so `hm up -- <tilt args>` forwards cleanly.
func tiltPassthrough(args []string) []string {
	if len(args) > 0 && args[0] == "--" {
		return args[1:]
	}
	return args
}

// runTilt runs tilt with the HM_* env added to the current environment.
func runTilt(env, args []string) error {
	path, err := exec.LookPath("tilt")
	if err != nil {
		return fmt.Errorf("tilt not found in PATH — install Tilt 0.36+ (https://docs.tilt.dev/install.html): %w", err)
	}
	cmd := exec.Command(path, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func printWarnings(resp ipc.Response) {
	for _, w := range resp.Warnings {
		fmt.Fprintf(os.Stderr, "hm: warning: %s\n", w)
	}
}

// serviceEntry / serviceList back the repeatable -service flag in `hm init`.
type serviceEntry struct {
	Name    string
	Default int
}

type serviceList []serviceEntry

func (s *serviceList) String() string {
	parts := make([]string, len(*s))
	for i, e := range *s {
		parts[i] = fmt.Sprintf("%s:%d", e.Name, e.Default)
	}
	return strings.Join(parts, ",")
}

func (s *serviceList) Set(v string) error {
	name, def, _ := strings.Cut(v, ":")
	if name == "" {
		return fmt.Errorf("empty service name in %q", v)
	}
	port := 0
	if def != "" {
		n, err := strconv.Atoi(def)
		if err != nil {
			return fmt.Errorf("bad default port %q for service %q", def, name)
		}
		port = n
	}
	*s = append(*s, serviceEntry{Name: name, Default: port})
	return nil
}

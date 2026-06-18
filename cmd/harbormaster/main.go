// Command harbormaster (alias hm) is the CLI for the harbormaster port authority.
//
// It is short-lived: it derives the current checkout's identity from git, talks
// to harbormasterd over a Unix socket, and prints or applies the resulting port
// lease. It is what humans, Tiltfiles, and other tools call. See SPEC.md for the
// full design.
package main

import (
	"errors"
	"fmt"
	"os"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

// errNotImplemented is returned by command stubs that are not wired up yet.
var errNotImplemented = errors.New("not implemented yet")

const usage = `harbormaster — the port authority for your local dev harbor (alias: hm)

Usage:
  hm <command> [arguments]

Commands:
  init       Register $PWD's repo as a project; write harbormaster.toml
  up         Resolve this checkout's ports and exec 'tilt up --port <tilt>'
  down       'tilt down' for this checkout; mark it inactive
  ports      Print this checkout's assigned ports (--json|--env|--write)
  ls         Dashboard of every project x worktree x ports x liveness (alias: ps)
  release    Free the lease for a checkout (default $PWD)
  prune      Reclaim leases whose worktree directory no longer exists
  doctor     Check daemon health, socket, pool headroom, squatters
  daemon     Run the daemon in the foreground (normally auto-started)

Flags:
  --version   Print version and exit
  -h, --help  Show this help

Run 'hm <command> -h' for command-specific help.
`

func main() {
	os.Exit(run(os.Args[1:]))
}

// run dispatches a subcommand and returns a process exit code.
func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	switch args[0] {
	case "-h", "--help", "help":
		fmt.Print(usage)
		return 0
	case "-V", "--version", "version":
		fmt.Printf("harbormaster %s\n", version)
		return 0
	}

	cmds := map[string]func([]string) error{
		"init":    cmdInit,
		"up":      cmdUp,
		"down":    cmdDown,
		"ports":   cmdPorts,
		"ls":      cmdLs,
		"ps":      cmdLs,
		"release": cmdRelease,
		"prune":   cmdPrune,
		"doctor":  cmdDoctor,
		"daemon":  cmdDaemon,
	}

	fn, ok := cmds[args[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "hm: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	if err := fn(args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "hm %s: %v\n", args[0], err)
		return 1
	}
	return 0
}

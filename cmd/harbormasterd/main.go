// Command harbormasterd is the harbormaster daemon: the single source of truth
// for which host port is leased to which (worktree, service). It listens on a
// Unix domain socket and speaks NDJSON — deliberately not a TCP port, because the
// thing that hands out scarce ports must not itself consume one. See SPEC.md.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	sock := socketPath()
	fmt.Printf("harbormasterd %s\n", version)
	fmt.Printf("socket: %s\n", sock)
	// TODO(phase-5): bind the Unix socket and serve NDJSON ops
	// (lease/list/release/prune/doctor). See docs/socket-protocol.md.
}

// socketPath resolves the daemon's Unix socket path:
// ${XDG_RUNTIME_DIR}/harbormaster/hm.sock, falling back to
// ~/.local/share/harbormaster/hm.sock when XDG_RUNTIME_DIR is unset (the macOS
// norm). See DECISIONS.md D3.
func socketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "harbormaster", "hm.sock")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".local", "share", "harbormaster", "hm.sock")
}

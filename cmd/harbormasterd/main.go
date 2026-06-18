// Command harbormasterd is the harbormaster daemon: the single source of truth
// for which host port is leased to which (worktree, service). It listens on a
// Unix domain socket and speaks NDJSON — deliberately not a TCP port, because the
// thing that hands out scarce ports must not itself consume one. See SPEC.md.
package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/venndata-org/harbormaster/internal/config"
	"github.com/venndata-org/harbormaster/internal/daemon"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cfg, err := config.LoadGlobal()
	if err != nil {
		log.Fatalf("harbormasterd: config: %v", err)
	}
	if err := daemon.Run(cfg, version); err != nil {
		if errors.Is(err, daemon.ErrAlreadyRunning) {
			fmt.Printf("harbormasterd already running at %s\n", cfg.Socket)
			return
		}
		log.Fatalf("harbormasterd: %v", err)
	}
}

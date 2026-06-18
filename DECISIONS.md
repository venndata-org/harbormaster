# DECISIONS

Engineering decisions made during the autonomous build that SPEC.md leaves open (or
that refine it). Newest first. If a decision changes the design, SPEC.md is amended
to match — these notes capture the _why_.

## 2026-06-18 — Phase 0

### D1 — CLI library: stdlib `flag` + a hand-rolled subcommand router

SPEC leaves the CLI lib open (cobra / urfave / stdlib `flag`). Chose stdlib `flag`
with a small map-based subcommand dispatcher. Rationale: the command surface is
small and stable, and the project values being dependency-light (SPEC §3). No
cobra/urfave to vendor or audit; per-command flags use `flag.FlagSet`. Revisit if
shell completions or nested commands push the complexity up.

### D2 — TOML parser: `github.com/BurntSushi/toml`

Config files are TOML (SPEC §7) and the stdlib has no TOML. Chose BurntSushi/toml —
the de-facto standard, with zero transitive dependencies. Planned to be the only
third-party runtime dependency for the MVP.

### D3 — Socket path under `XDG_RUNTIME_DIR` is namespaced

SPEC §3 writes the default as
`${XDG_RUNTIME_DIR:-~/.local/share/harbormaster}/hm.sock`. When `XDG_RUNTIME_DIR`
is set we place the socket at `$XDG_RUNTIME_DIR/harbormaster/hm.sock` (a namespaced
subdir) rather than the runtime root, matching how the state and config dirs are
namespaced. The fallback branch (`XDG_RUNTIME_DIR` unset, the macOS norm) stays
exactly `~/.local/share/harbormaster/hm.sock`, per the SPEC `config.toml` example.

### D4 — Berth offsets assigned by request order

Within a block, `tilt` = `block_base + 0` and the i-th requested service =
`block_base + 1 + i`. The CLI sends `services` ordered by their configured `offset`
(harbormaster.toml `[services]`). For the common contiguous case (offsets `1..N`,
as in the SPEC worked example) this reproduces the configured ports exactly.
Non-contiguous explicit offsets and per-worktree pins (SPEC §4.4) are deferred to
the roadmap; the wire format stays the plain `services: [...]` shape from SPEC §8.

### D6 — Project name: config wins; git default handles the `/main` layout

SPEC §6 says `project = basename(main worktree toplevel)`. Taken literally that
yields **"main"** for the opera layout (`~/dev-vd/<project>/main`) — which is
exactly how this repo and the SPEC worked example are laid out, so every project
would be named "main". Resolution:

1. The authoritative project name is the `name` field in `harbormaster.toml` (SPEC
   §7 shows `name = "groundtruth"`); the CLI prefers it. It is committed, so all
   worktrees agree.
2. The git-derived value in `internal/gitident` is only a **default** (used by
   `hm init` and when no config exists). To make that default useful under the
   opera layout, when the main worktree's basename is the generic `main`/`master`
   we use the containing directory's name instead.

This keeps allocation correct regardless (the key is the instance path, not the
project name) and makes the project label sensible. SPEC §6 should be read with
this refinement.

### D5 — Allocator takes an injectable port prober

`internal/alloc` will depend on a `Prober` func (`port -> free?`) instead of binding
sockets directly. Production uses a real TCP bind-probe on `127.0.0.1`; unit tests
inject a deterministic fake. This lets us test the deterministic block math
(non-overlap, stability, reserved-port skip) without touching real sockets, while
the e2e smoke test exercises real binding.

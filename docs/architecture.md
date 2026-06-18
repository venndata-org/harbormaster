# Architecture

A map of the moving parts. The authoritative design is [`SPEC.md`](../SPEC.md);
this doc orients you in the code.

## Components

```
   you / Tilt        harbormasterd (daemon, long-running)
   ┌────────┐  uds   ┌─────────────────────────────────────────────┐
   │  hm    │ ─────▶ │ • owns the lease table (state.json)         │
   │ (CLI)  │ NDJSON │ • assigns blocks/berths, bind-probes ports  │
   └────────┘ ◀───── │ • persists atomically, replies              │
       │             │ • listens on a Unix socket (no TCP port)    │
       │ exec/env    └─────────────────────────────────────────────┘
       ▼                              │
   ┌────────┐                  [ state.json ]
   │  tilt  │  reads HM_PORT_* env / `hm ports --json`
   └────────┘
```

- **`harbormaster` / `hm` (CLI, `cmd/harbormaster`)** — short-lived. Derives the
  current checkout's identity from git, connects to the daemon, and prints or
  applies the lease. Auto-starts the daemon if its socket is dead.
- **`harbormasterd` (daemon, `cmd/harbormasterd`)** — long-running, single source
  of truth. Holds the lease table in memory, persists it to `state.json`, and
  answers NDJSON requests over a Unix socket.

## Internal packages

| Package | Responsibility |
|---------|----------------|
| `internal/config` | Resolve XDG paths; load global `config.toml` + per-project `harbormaster.toml`. |
| `internal/gitident` | Derive `project` / `instance` / `label` from `$PWD` via git (worktree-aware). |
| `internal/alloc` | Deterministic block allocator: lowest-free block, berth offsets, reserved-port skip, injectable bind-probe. The heart of the tool. |
| `internal/state` | Load/save `state.json` atomically (temp file + rename). |
| `internal/ipc` | NDJSON message types + Unix-socket client and server. |

## Identity (worktree-aware)

Derived from git in `$PWD`:

- **project** = basename of the git *common dir's* top-level (so every worktree of
  a repo maps to the same project).
- **instance** = `git rev-parse --show-toplevel` (this checkout's absolute path) —
  the allocation key.
- **label** = `git branch --show-current` (the human-friendly name).

## Allocation (summary)

- A machine-wide pool, default `[20000, 32000)`, divided into fixed-size **blocks**
  (default 20 ports).
- Each instance gets the **lowest free block** on first lease; the base is persisted
  and stable until released/pruned. Blocks never overlap between live instances.
- Within a block: offset 0 is the Tilt UI port; requested services take offsets
  1, 2, 3, … in request order.
- Before returning a port the daemon **bind-probes** `127.0.0.1:port` and skips
  reserved ports, so assignments stay truthful, not just bookkept.

See [`socket-protocol.md`](./socket-protocol.md) for the wire format and
[`tilt-integration.md`](./tilt-integration.md) for how ports reach Tiltfiles.

## Paths (XDG)

| What | Path |
|------|------|
| Socket | `${XDG_RUNTIME_DIR}/harbormaster/hm.sock` or `~/.local/share/harbormaster/hm.sock` |
| State | `${XDG_STATE_HOME:-~/.local/state}/harbormaster/state.json` |
| Global config | `${XDG_CONFIG_HOME:-~/.config}/harbormaster/config.toml` |
| Project config | `harbormaster.toml` at the repo root (committed) |

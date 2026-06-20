# harbormaster CLI (`hm`) — command reference

`harbormaster` (alias `hm`) is short-lived: it derives the current checkout's
identity from git in `$PWD`, talks to the daemon over a Unix socket, and prints or
applies the result. The **daemon auto-starts** on first use (the CLI re-execs
itself as `hm daemon`, detached) — you never start it by hand.

Identity per invocation: `instance` = this worktree's toplevel (`git rev-parse
--show-toplevel`), `project` = `harbormaster.toml` `name` (else derived from git),
`label` = current branch.

## Commands

### `hm init [--name NAME] [--service NAME:DEFAULT ...] [--force]`
Write a starter `harbormaster.toml` at the repo root (commit it). `--service` is
repeatable; each gets a stable block offset in declaration order. `--name`
overrides the derived project name. `--force` overwrites an existing file.
```sh
hm init --service web:3000 --service api:4000 --service postgres:5432
```

### `hm up [-- <tilt args>]`
Lease this checkout's ports, export `HM_TILT_PORT` + `HM_PORT_*`, and exec
`tilt up --port <tilt> <tilt args>`. Auto-registers the instance on first use.
```sh
hm up
hm up -- --stream --hud=false
```

### `hm down [-- <tilt args>]`
Run `tilt down` with the same `HM_*` env. Keeps the lease (the block stays stable);
use `hm release` to free it.

### `hm ports [--json | --env | --write]`
Print this checkout's berths. Read-only when a lease already exists (it never
re-assigns an instance's ports just because its Tilt is running); allocates only on
first use.
- *(default)* human table
- `--json` — `{instance, project, label, tilt, block, ports}`
- `--env` — shell lines: `HM_TILT_PORT=…`, `HM_PORT_WEB=…`
- `--write` — write `.harbormaster.env` (gitignored) at the repo root
```sh
eval "$(hm ports --env)"     # load ports into the current shell
```

### `hm ls` (alias `hm ps`)
Table of every leased instance: project, label, block, tilt port, services, path.

### `hm release [PATH]`
Free a lease. Default: this checkout (`$PWD`). Pass a path to release another
(useful for a worktree you're about to delete).

### `hm prune`
Reclaim leases whose worktree directory no longer exists (e.g. removed worktrees).

### `hm doctor`
Daemon health, socket/state paths, pool range + headroom (free blocks), and leased
ports currently in use. (In the MVP, "in use" includes a lease's own Tilt while it
runs — liveness tracking is roadmap.)

### `hm daemon`
Run the daemon in the foreground (normally auto-started/detached). For debugging or
running under a supervisor.

### Global flags
`hm --version`, `hm -h | --help`, `hm <cmd> -h`.

## Notes

- Commands that need identity (`init`, `up`, `down`, `ports`, no-arg `release`)
  must run inside a git repo. Daemon-global commands (`ls`, `prune`, `doctor`) work
  anywhere.
- Config: global `~/.config/harbormaster/config.toml` (`pool_start`, `pool_end`,
  `block_size`, `reserved`, `socket`, `state`); per-project `harbormaster.toml` at
  the repo root (committed).
- Exit status is non-zero on error; warnings (e.g. a relocated port) print to stderr.

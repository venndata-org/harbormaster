# CLAUDE.md

Guidance for AI agents working in this repo.

## What this is

harbormaster — a port authority for local [Tilt](https://tilt.dev) dev. A daemon
(`harbormasterd`) leases each git **worktree** a stable, conflict-free block of
host ports; a short-lived CLI (`harbormaster`, alias `hm`) is what humans and
Tiltfiles call. The allocation key is `(worktree_absolute_path, service)`, which is
what lets N worktrees of one repo all `tilt up` at once.

## Read order (every fresh context)

1. **`SPEC.md`** — the full design and the source of truth. Code never gets ahead
   of it: if code and SPEC disagree, the SPEC wins until a human amends it (and the
   amendment is logged in `DECISIONS.md`).
2. **`PROGRESS.md`** — the MVP checklist and current state.
3. **`DECISIONS.md`** — non-obvious engineering calls made during the build.
4. **`.claude/skills/harbormaster/`** — the shipped Claude Code skill.

## Build / test

```sh
make build      # -> bin/harbormaster, bin/harbormasterd, bin/hm
make test       # go test ./...
make vet        # go vet ./...
go build ./...  # quick compile check
```

**Verification gate** before any commit: `go build ./...`, `go vet ./...`, and
`go test ./...` must all be green. `internal/alloc` is the heart of the tool and
must stay heavily unit-tested.

## Layout

- `cmd/harbormaster` — CLI entrypoint + subcommands.
- `cmd/harbormasterd` — daemon entrypoint.
- `internal/config` — global `config.toml` + per-project `harbormaster.toml` (TOML).
- `internal/gitident` — git-derived project / instance / label (worktree-aware).
- `internal/alloc` — deterministic block allocator + bind-probe (unit-tested).
- `internal/state` — atomic `state.json` load/save.
- `internal/ipc` — Unix-socket NDJSON client/server + message types.
- `docs/` — architecture, socket protocol, Tilt integration.

## Conventions

- **Dependency-light.** Stdlib for the socket / NDJSON / state; only
  `github.com/BurntSushi/toml` for config parsing.
- **Localhost-only.** The Unix socket is the only listener; no TCP ports.
- **Conventional Commits.** Single-user, single-host. No auth / multi-tenant (that
  is roadmap, not MVP).

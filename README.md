# harbormaster

**The port authority for your local dev harbor.** Run every project's `tilt up`
at once — including multiple git worktrees of the *same* project — without ports
ever colliding.

> Status: **functional MVP in progress.** The daemon, allocator, CLI, and Tilt
> env-var integration work today — `hm up`, `hm ports`, `hm ls`, `hm release`,
> `hm doctor` all run, and two worktrees of one repo already get conflict-free
> port blocks. The bundled Claude Code skill is the last MVP piece. The full design
> lives in [`SPEC.md`](./SPEC.md); progress is tracked in [`PROGRESS.md`](./PROGRESS.md).

## Why

[Tilt](https://tilt.dev) Tiltfiles hardcode host ports — the Tilt UI (`10350`),
host services (`web:3000`, `api:4000`), compose-published ports (`postgres:5432`).
Two `tilt up`s at once collide. Two **worktrees of the same repo** collide on
everything. harbormaster assigns each checkout its own stable, conflict-free block
of ports so they all run simultaneously.

## How it works

A tiny daemon (`harbormasterd`) is the single source of truth for port leases and
listens on a **Unix socket** (the thing that hands out scarce ports shouldn't hog
one). A short-lived CLI (`harbormaster`, alias `hm`) is what you — and your
Tiltfiles — call. Each *checkout* (worktree) gets a contiguous, persisted port
**block**; services get stable offsets within it.

```sh
hm up            # leases a block for $PWD, exports HM_PORT_*, runs `tilt up --port <assigned>`
hm ls            # dashboard of every project × worktree × ports × liveness
hm ports --json  # machine-readable berths for the current checkout
```

A retrofitted Tiltfile just reads env vars (and keeps working without harbormaster
installed, via defaults):

```python
def hm_port(name, default):
    return int(os.getenv('HM_PORT_' + name.upper(), str(default)))
WEB = hm_port('web', 3000)
```

See [`SPEC.md`](./SPEC.md) for the allocation model, protocol, and config.

## Claude Code skill

This repo ships a [Claude Code](https://claude.com/claude-code) skill
(`.claude/skills/harbormaster/`) that teaches an agent to install harbormaster,
register a project, and retrofit its Tiltfile — plus a working reference for the
Tilt CLI itself.

## Install

```sh
go install github.com/venndata-org/harbormaster/cmd/harbormaster@latest
go install github.com/venndata-org/harbormaster/cmd/harbormasterd@latest
# or, from a clone:
make install
```

## License

MIT — see [`LICENSE`](./LICENSE).

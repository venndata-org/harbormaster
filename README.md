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

## Usage

**[docs/usage.md](./docs/usage.md)** is the step-by-step guide: install, `hm init`,
retrofit a Tiltfile, `hm up`, and running multiple worktrees at once. See
[`SPEC.md`](./SPEC.md) for the allocation model, protocol, and config.

## For coding agents

harbormaster is built to be driven by an AI agent. Three on-ramps:

1. **A [Claude Code](https://claude.com/claude-code) skill** at
   `.claude/skills/harbormaster/` — `SKILL.md` plus references (CLI, Tilt CLI,
   Tiltfile retrofit, allocation model, socket protocol). It teaches an agent to
   install harbormaster, run `hm init`, retrofit a Tiltfile to read `HM_PORT_*`,
   and launch with `hm up`. Install it so agents can use it in **any** project:

   ```sh
   make install-skill        # -> ~/.claude/skills/harbormaster (all your projects)
   # or vendor it into one repo:
   make install-skill SKILL_DIR=/path/to/repo/.claude/skills/harbormaster
   ```

2. **Machine-readable CLI** — `hm ports --json` returns
   `{instance, project, label, tilt, block, ports}` for any tool to consume.

3. **The daemon's NDJSON socket protocol** — for non-Claude agents, talk to
   `harbormasterd` directly over its Unix socket (see
   [`docs/socket-protocol.md`](./docs/socket-protocol.md)).

## Install

```sh
go install github.com/venndata-org/harbormaster/cmd/harbormaster@latest
go install github.com/venndata-org/harbormaster/cmd/harbormasterd@latest
# or, from a clone:
make install
```

## License

MIT — see [`LICENSE`](./LICENSE).

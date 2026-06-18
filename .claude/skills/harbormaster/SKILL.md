---
name: harbormaster
description: >-
  Install and use harbormaster (CLI `hm`) to give each git checkout/worktree a
  stable, conflict-free block of host ports for Tilt. Use when a project's
  `tilt up` collides with another, when running multiple git worktrees of one repo
  at once, when retrofitting a Tiltfile to read assigned ports (HM_PORT_*), or when
  debugging local Tilt host-port collisions.
---

# harbormaster

harbormaster is a port authority for local [Tilt](https://tilt.dev) dev. A daemon
(`harbormasterd`) leases each git **worktree** a stable, conflict-free block of host
ports; the CLI (`harbormaster`, alias `hm`) is what you and Tiltfiles call. The
allocation key is `(worktree_path, service)`, so N worktrees of one repo can all
`tilt up` simultaneously without colliding.

Use this skill to install harbormaster, register a project, retrofit its Tiltfile
to consume assigned ports, and run it.

> **Retrofitting a whole project in one shot?** Hand an agent the ready-made seed
> prompt in [references/retrofit-prompt.md](references/retrofit-prompt.md) — it
> walks through reserving ports, checking for conflicts, and updating the infra.

## When to use

- A Tiltfile hardcodes host ports (Tilt UI `10350`, web `3000`, api `4000`,
  postgres `5432`, …) and two `tilt up`s — or two worktrees — collide.
- You want every project (and every branch worktree) running at once.
- You're setting up a new project or retrofitting an existing Tiltfile.

## Install

From a clone of `github.com/venndata-org/harbormaster`:

```sh
make install            # builds + installs harbormaster, harbormasterd, and `hm`
# or:
go install github.com/venndata-org/harbormaster/cmd/harbormaster@latest
go install github.com/venndata-org/harbormaster/cmd/harbormasterd@latest
```

`hm` is an alias for `harbormaster`. The daemon **auto-starts** on first use (the
CLI re-execs itself as `hm daemon`, detached) — never start it by hand.

## Core workflow

1. **Register the project** — writes `harbormaster.toml` at the repo root (commit it):

   ```sh
   cd <repo>
   hm init --service web:3000 --service api:4000 --service postgres:5432
   ```

   Each `--service name:default` becomes a service with a stable block offset; the
   default is the legacy hardcoded port (used when harbormaster isn't installed).

2. **Retrofit the Tiltfile** to read assigned ports — full procedure in
   [references/tiltfile-retrofit.md](references/tiltfile-retrofit.md). The essence:

   ```python
   def hm_port(name, default):
       return int(os.getenv('HM_PORT_' + name.upper(), str(default)))

   WEB = hm_port('web', 3000)
   API = hm_port('api', 4000)
   ```

   Defaults preserve standalone behavior when harbormaster is absent.

3. **Run with `hm up`** instead of `tilt up`:

   ```sh
   hm up                 # leases ports, exports HM_*, execs `tilt up --port <tilt>`
   hm up -- --stream     # args after -- are forwarded to tilt
   ```

   In a second worktree of the same repo, `hm up` gets a different block — both run
   at once, no collisions.

## Inspect / manage

```sh
hm ports            # this checkout's assigned ports (human)
hm ports --json     # machine-readable
hm ls               # every project x worktree x ports
hm doctor           # daemon health, pool headroom, in-use ports
hm release          # free this checkout's lease
hm prune            # reclaim leases for deleted worktrees
```

## References

Read these as needed (they keep this file short):

- [references/retrofit-prompt.md](references/retrofit-prompt.md) — **seed prompt** to drive a full project retrofit (reserve ports, check conflicts, update infra).
- [references/cli-commands.md](references/cli-commands.md) — every `hm` subcommand + flags.
- [references/tilt-cli.md](references/tilt-cli.md) — the Tilt CLI itself (up/down/args/ports).
- [references/tiltfile-retrofit.md](references/tiltfile-retrofit.md) — step-by-step Tiltfile retrofit.
- [references/allocation-model.md](references/allocation-model.md) — how ports are assigned.
- [references/socket-protocol.md](references/socket-protocol.md) — daemon NDJSON protocol.

# Using harbormaster

harbormaster gives every git checkout its own stable, conflict-free block of host
ports, so you can run multiple `tilt up`s — and multiple **worktrees of the same
repo** — at the same time without editing ports by hand.

You interact with one command: `hm` (alias for `harbormaster`). The background
daemon starts itself the first time you run anything — you never manage it.

---

## Install

From a clone of the repo:

```sh
make install        # builds + installs `harbormaster`, `harbormasterd`, and `hm`
```

Or with Go directly:

```sh
go install github.com/venndata-org/harbormaster/cmd/harbormaster@latest
go install github.com/venndata-org/harbormaster/cmd/harbormasterd@latest
```

Make sure your Go bin dir (`go env GOPATH`/bin, or `GOBIN`) is on `PATH`. Check it:

```sh
hm --version
```

---

## Quickstart

```sh
cd my-project
hm init --service web:3000 --service api:4000   # declare your services once
# ...retrofit the Tiltfile to read HM_PORT_* (see step 2)...
hm up                                           # run it (instead of `tilt up`)
```

That's the whole loop. The rest of this page explains each step.

---

## 1. Register your project

Run this once per repo, at its root:

```sh
hm init --service web:3000 --service api:4000 --service postgres:5432
```

This writes a `harbormaster.toml` describing your services and their *legacy*
ports (the ones currently hardcoded in your Tiltfile):

```toml
name = "my-project"

[services]
web      = { offset = 1, default = 3000 }
api      = { offset = 2, default = 4000 }
postgres = { offset = 3, default = 5432 }
```

**Commit this file.** Teammates who clone get the same service layout; each
machine assigns its own locally-free ports.

> No services yet? `hm init` with no `--service` flags writes a template with a
> commented example you can fill in.

---

## 2. Make the Tiltfile read the assigned ports

harbormaster hands ports to Tilt through environment variables:

- `HM_TILT_PORT` — the Tilt web UI port
- `HM_PORT_WEB`, `HM_PORT_API`, … — one per service (name uppercased)

Add this helper near the top of your `Tiltfile` and use it wherever a host port was
hardcoded:

```python
def hm_port(name, default):
    return int(os.getenv('HM_PORT_' + name.upper(), str(default)))

WEB = hm_port('web', 3000)
API = hm_port('api', 4000)
```

The `default` keeps the Tiltfile working for anyone who hasn't installed
harbormaster. The Tilt **UI** port needs no Tiltfile change — `hm up` passes it via
`tilt up --port`.

For the exact before/after edits (`local_resource`, `port_forwards`,
`docker_compose`), see [tilt-integration.md](./tilt-integration.md).

---

## 3. Launch with `hm up`

Use `hm up` instead of `tilt up`:

```sh
hm up                  # leases ports, exports HM_*, runs `tilt up --port <tilt>`
hm up -- --stream      # anything after `--` is forwarded to tilt
```

To stop:

```sh
hm down                # runs `tilt down` (your lease is kept, so ports stay stable)
```

---

## Running multiple worktrees at once

This is the whole point. Say you use worktrees per branch:

```sh
# worktree 1
cd ~/dev/my-project/main   && hm up      # gets block 20000–20019 (tilt 20000, web 20001, …)

# worktree 2, at the same time
cd ~/dev/my-project/feat-x && hm up      # gets block 20020–20039 (tilt 20020, web 20021, …)
```

Both stacks run simultaneously, on different ports, with **zero** manual edits.
Each worktree keeps its block across restarts.

---

## Everyday commands

| Command | What it does |
|---|---|
| `hm ports` | Show this checkout's assigned ports. |
| `hm ports --json` | Same, machine-readable. |
| `hm ports --env` | Print `HM_TILT_PORT=…` / `HM_PORT_*=…` lines. |
| `hm ports --write` | Write `.harbormaster.env` (gitignored) at the repo root. |
| `hm ls` (or `hm ps`) | Every project × worktree × ports, in a table. |
| `hm doctor` | Daemon health, pool headroom, ports in use. |
| `hm release` | Free this checkout's lease. |
| `hm release <path>` | Free another checkout's lease. |
| `hm prune` | Reclaim leases for worktrees that were deleted. |
| `hm daemon` | Run the daemon in the foreground (rarely needed). |

Load ports into your shell without Tilt:

```sh
eval "$(hm ports --env)"
echo "$HM_PORT_WEB"
```

Example `hm ls`:

```
PROJECT     LABEL   BLOCK        TILT   SERVICES             PATH
my-project  main    20000-20019  20000  web:20001 api:20002  /Users/you/dev/my-project/main
my-project  feat-x  20020-20039  20020  web:20021 api:20022  /Users/you/dev/my-project/feat-x
```

---

## Configuration

**Per project** — `harbormaster.toml` at the repo root (committed). Written by
`hm init`; edit to add services or change defaults.

**Global** — `~/.config/harbormaster/config.toml` (optional; sensible defaults if
absent):

```toml
pool_start = 20000      # first port in the pool
pool_end   = 32000      # one past the last (half-open)
block_size = 20         # ports reserved per checkout
reserved   = [5432, 6379]   # never hand these out
```

Defaults: pool `[20000, 32000)`, 20 ports per block (so ~600 worktrees fit).

---

## Troubleshooting

- **`hm: not a git repository`** — `init`, `up`, `down`, `ports`, and bare
  `release` derive identity from git in the current directory. Run them inside the
  repo. (`ls`, `prune`, `doctor` work anywhere.)
- **`tilt not found in PATH`** — install [Tilt](https://docs.tilt.dev/install.html)
  0.36+. harbormaster assigns the ports; Tilt runs the stack.
- **A port shows up as "in use" in `hm doctor`** — that's expected while that
  checkout's Tilt is running. It only matters for a checkout you believe is idle.
- **Daemon won't start** — see its log at
  `~/.local/state/harbormaster/harbormasterd.log`. You can also run `hm daemon` in
  the foreground to watch it directly.
- **Stale leases after deleting worktrees** — run `hm prune`.

---

## How ports are chosen (in one line)

Each checkout gets the **lowest free block**; within it, the Tilt UI takes offset 0
and your services take offsets 1, 2, 3, … in declared order. Ports are bind-probed
before they're handed out, and blocks never overlap between live checkouts. Details
in [the allocation model](../.claude/skills/harbormaster/references/allocation-model.md)
and `SPEC.md` §4.

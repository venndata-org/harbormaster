# harbormaster — Specification

> **Status:** design + scaffold. This document is the source of truth for the
> autonomous build. Implementation lives behind it, not ahead of it: if code and
> SPEC disagree, the SPEC wins until a human amends it. Material design changes
> made during the build go in `DECISIONS.md`, not silently into code.

---

## 1. The problem

Local development on a fleet of services uses [Tilt](https://tilt.dev) (`tilt up`).
Every Tiltfile binds **host ports**:

- **Tilt's own web UI** — `tilt up --port N` (default `10350`).
- **Host `local_resource` servers** — e.g. api `:4000`, web `:3000`.
- **docker-compose published ports** — e.g. postgres `:5432`, grafana `:3001`.

Running more than one project's `tilt up` at the same time collides on these
ports. Worse: running **two git worktrees of the *same* project** (the normal
state of affairs when you use [opera](https://github.com/opera-org/opera) to spin
up a worktree per branch) collides on *every* port, because both checkouts share
the same Tiltfile and therefore the same hardcoded numbers.

Today the workaround is hand-editing ports per checkout. It does not scale, it is
not stable across restarts, and it makes "run everything at once" impractical.

## 2. The idea

A **harbormaster** is the port authority for a harbor: it assigns each arriving
ship a **berth** so no two ships occupy the same slip. This tool is exactly that,
for local dev ports.

- A small **daemon** (`harbormasterd`) is the single source of truth for which
  port is leased to whom. It listens on a **Unix socket** — deliberately *not* a
  TCP port, because the thing that hands out scarce ports must not itself consume
  one.
- A short-lived **CLI** (`harbormaster`, alias `hm`) talks to the daemon, and is
  what humans, Tiltfiles, and other tools call.
- A **Claude Code skill** teaches an agent to install harbormaster, register a
  project, and retrofit that project's Tiltfile to consume assigned ports — plus
  carries a working reference for the Tilt CLI itself.

### Core nouns

| Term | Meaning |
|------|---------|
| **project** | A git repository, identified by a stable name (default: repo basename). |
| **instance** | One *checkout* of a project — i.e. one worktree. Keyed by the absolute path of the checkout. The branch/dir name is its human label. |
| **service** | A named port consumer within a project, e.g. `web`, `api`, `postgres`, plus the implicit `tilt` UI port. |
| **berth** | One leased port for one `(instance, service)` pair. |
| **block** | A contiguous range of ports reserved for one instance, so its berths are grouped and predictable. |

The allocation key is **`(instance_path, service)`** — *not* `(project, service)`.
That is the whole reason multiple worktrees of one repo can run at once.

## 3. Architecture

```
                 ┌─────────────────────────────────────────────┐
   you / Tilt    │                                             │
   ┌────────┐    │   harbormasterd  (daemon, long-running)     │
   │  hm    │───▶│   • owns the lease table (state.json)       │
   │ (CLI)  │ uds│   • bind-probes candidate ports             │
   └────────┘    │   • assigns blocks/berths, persists, replies│
       ▲         │   • listens on ~/.local/share/hm/hm.sock    │
       │         └─────────────────────────────────────────────┘
       │ exec/env                         │ NDJSON over Unix socket
       ▼                                  ▼
   ┌────────┐                       [ state.json ]
   │  tilt  │  reads HM_PORT_* env / `hm ports --json`
   └────────┘
```

- **Transport:** Unix domain socket, **NDJSON** request/response (one JSON object
  per line), mirroring opera's proven `socket.rs` protocol. Default socket path
  `${XDG_RUNTIME_DIR:-~/.local/share/harbormaster}/hm.sock`.
- **Persistence:** `state.json` written atomically (temp file + rename) on every
  mutation, under `${XDG_STATE_HOME:-~/.local/state}/harbormaster/`. Reloaded on
  daemon start.
- **Auto-start:** the CLI starts the daemon if the socket is dead (connect →
  `ENOENT`/`ECONNREFUSED` → spawn `harbormasterd` detached → retry). Same UX as
  opera.
- **Single-user, single-host** for v1. Multi-user is out of scope (see §10).

## 4. Allocation model

Hybrid **deterministic-block + dynamic-probe**, persisted for stability.

1. **Pool.** A configurable global range, default `[20000, 32000)`. Chosen to sit
   above common app ports and below the macOS ephemeral range (`49152+`), so we
   neither fight real services nor the kernel's outbound port picker.
2. **Block per instance.** On an instance's first registration it is granted a
   contiguous block of `block_size` ports (default `20`). The block base is
   persisted and **stable forever** until the lease is released or pruned.
   - `block[0]` is reserved for the **Tilt UI port**.
   - services get deterministic offsets within the block (declaration order, or
     pinned offsets from project config).
3. **Bind-probe before commit.** Before returning any port, the daemon attempts
   to `bind` `127.0.0.1:port` (and releases immediately). If a previously-leased
   port is now squatted by an external process, the daemon reallocates that berth
   and records a warning. This keeps assignments truthful, not just bookkept.
4. **Pins (optional).** A project may pin a service to a fixed port for a given
   instance (e.g. the `main` worktree always serves `web` on `3000`). The daemon
   honors a pin when free and routes other instances around it.
5. **Determinism.** Same set of instances + same config ⇒ same assignment. Blocks
   never overlap between live instances. Worktree A in block `20000–20019`,
   worktree B in `20020–20039`, both `tilt up` at once, zero contention.

### Worked example

Two worktrees of `groundtruth` (`web`, `api`, `postgres`, `grafana`):

```
groundtruth @ ~/dev-vd/groundtruth/main      block base 20000
  tilt=20000  web=20001  api=20002  postgres=20003  grafana=20004
groundtruth @ ~/dev-vd/groundtruth/feat-x    block base 20020
  tilt=20020  web=20021  api=20022  postgres=20023  grafana=20024
```

## 5. Tiltfile integration

Two layers, lowest-friction first.

### 5a. Environment variables (primary, zero-dependency)

`harbormaster up` (or `hm ports --env`) exports a stable, predictable env for the
current instance:

```
HM_TILT_PORT=20000
HM_PORT_WEB=20001
HM_PORT_API=20002
HM_PORT_POSTGRES=20003
```

A retrofitted Tiltfile reads them with a tiny helper:

```python
def hm_port(name, default):
    return int(os.getenv('HM_PORT_' + name.upper(), str(default)))

WEB  = hm_port('web', 3000)
API  = hm_port('api', 4000)
```

Defaults preserve the old hardcoded behavior when harbormaster is absent, so the
Tiltfile still works for someone who hasn't installed the tool.

### 5b. `harbormaster up` wrapper (handles Tilt's own port)

```
hm up [-- <extra tilt args>]
```

resolves the lease for `$PWD`, exports `HM_*`, and execs:

```
tilt up --port "$HM_TILT_PORT" <extra tilt args>
```

So the one command a developer learns is `hm up` instead of `tilt up`. `hm down`
runs `tilt down` and marks the instance inactive.

### 5c. Tilt Starlark extension (roadmap, §10)

A loadable `ext://harbormaster` exposing `hm = harbormaster()` and `hm.port('web')`
for projects that prefer in-Tiltfile resolution over env vars.

## 6. CLI surface

| Command | Purpose |
|---------|---------|
| `hm init` | Interactive: register `$PWD`'s repo as a project, declare services, optional pins; write `harbormaster.toml` at repo root (committed to git). |
| `hm up [-- <tilt args>]` | Resolve lease for this instance, export `HM_*`, exec `tilt up --port <tilt>`. Auto-registers instance on first use. |
| `hm down` | `tilt down` for this instance; mark inactive. |
| `hm ports [--json\|--env\|--write]` | Resolve & print this instance's berths. `--write` drops a gitignored `.harbormaster.env`. |
| `hm ls` / `hm ps` | Dashboard: every project × instance × berths × live/idle. |
| `hm release [path]` | Free the lease for an instance (default `$PWD`). |
| `hm prune` | Reclaim leases whose worktree directory no longer exists. |
| `hm doctor` | Check daemon health, socket, pool headroom, external squatters. |
| `hm daemon` | Run the daemon in the foreground (normally auto-started/detached). |

Instance/project identity is derived from git in `$PWD`:
`project = basename(git rev-parse --show-toplevel of the main worktree)` (the
common dir, so worktrees map to the same project), `instance = git rev-parse
--show-toplevel` (this checkout), `label = git branch --show-current`.

## 7. Configuration

### Global — `${XDG_CONFIG_HOME:-~/.config}/harbormaster/config.toml`

```toml
pool_start = 20000
pool_end   = 32000
block_size = 20
reserved   = [5432, 6379]   # never hand these out
socket     = "~/.local/share/harbormaster/hm.sock"
state      = "~/.local/state/harbormaster/state.json"
```

### Per-project — `harbormaster.toml` at repo root (committed)

```toml
name = "groundtruth"

[services]
web      = { offset = 1, default = 3000 }
api      = { offset = 2, default = 4000 }
postgres = { offset = 3, default = 5432 }
grafana  = { offset = 4, default = 3001 }

# Optional: pin a service to a fixed port for a specific worktree label.
[pins.main]
web = 3000
```

Committing this means a teammate who clones gets the same service topology; their
*local* daemon assigns locally-free blocks. Ports are coordinated per-machine, not
shared across machines.

## 8. Daemon protocol (NDJSON over Unix socket)

Request and response are single-line JSON objects. Sketch (the build refines exact
field names and records them in `docs/socket-protocol.md`):

```jsonc
// → lease request
{"op":"lease","instance":"/Users/av/dev-vd/groundtruth/feat-x","project":"groundtruth","label":"feat-x","services":["web","api","postgres"]}
// ← reply
{"ok":true,"tilt":20020,"ports":{"web":20021,"api":20022,"postgres":20023},"block":[20020,20039]}

{"op":"list"}                          // → ; ← {"ok":true,"instances":[...]}
{"op":"release","instance":"..."}      // → ; ← {"ok":true}
{"op":"prune"}                         // → ; ← {"ok":true,"reclaimed":[...]}
{"op":"doctor"}                        // → ; ← {"ok":true,"squatters":[...],"headroom":N}
```

## 9. State shape (`state.json`)

```jsonc
{
  "version": 1,
  "instances": {
    "/Users/av/dev-vd/groundtruth/feat-x": {
      "project": "groundtruth",
      "label": "feat-x",
      "block": [20020, 20039],
      "berths": {"tilt": 20020, "web": 20021, "api": 20022},
      "createdAt": "<iso8601>",
      "lastSeenAt": "<iso8601>"
    }
  }
}
```

## 10. Roadmap — MVP → dream big

**MVP (build target for the autonomous loop):**

- [ ] `harbormasterd`: Unix socket, NDJSON, lease table, atomic `state.json`.
- [ ] Deterministic block allocation + bind-probe + reserved ports.
- [ ] CLI: `init`, `up`, `down`, `ports`, `ls`, `release`, `prune`, `doctor`, `daemon`.
- [ ] CLI auto-starts daemon.
- [ ] Git-derived project/instance identity (worktree-aware).
- [ ] Env-var Tilt integration + `hm up` wrapper.
- [ ] The Claude Code skill (overview, CLI ref, Tilt CLI ref, retrofit procedure).
- [ ] `go build ./...`, `go vet`, unit tests for the allocator, one e2e smoke test
      (two fake instances → assert non-overlapping blocks & ports).

**Dream big (documented; not required for v1):**

- Tilt Starlark extension (`ext://harbormaster`).
- Live TUI dashboard (`hm ls --watch`) with heartbeat liveness.
- docker-compose port rewriting (publish assigned host ports automatically).
- **opera integration:** opera creates a worktree → harbormaster auto-leases a
  block for it, so a new branch session is born with conflict-free ports.
- Homebrew tap + `go install` distribution; shell completions.
- Pinned/preferred ports honored across instances.
- `hm doctor --heal` to evict external squatters / reallocate automatically.
- Optional HTTP shim for non-shell consumers.

## 11. Non-goals (v1)

- Cross-machine / multi-user coordination.
- Managing container *internal* ports (only host-published ports matter).
- Being a process supervisor — Tilt already supervises; harbormaster only assigns.
- Replacing Tiltfiles — it feeds them, it does not generate them.

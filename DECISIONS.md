# DECISIONS

Engineering decisions made during the autonomous build that SPEC.md leaves open (or
that refine it). Newest first. If a decision changes the design, SPEC.md is amended
to match — these notes capture the _why_.

## 2026-06-20 — fix: instance self-conflict

### D10 — ownership-aware probe + read-only `hm ports`

**Bug:** re-leasing an instance bind-probed every berth, and a port held by the
instance's *own* running Tilt was indistinguishable from an external squatter — so
the lease relocated the berths and persisted the new numbers. After `hm up`, a
later `hm ports`/`hm up` in that dir reported and saved *different* ports than Tilt
was serving ("conflicting with itself"). The block base stayed stable; only the
within-block offsets churned (relocated while running, restored when stopped).

**Fix (two parts):**

1. **Ownership-aware probe (alloc).** `alloc.Request` carries `Own` — the set of
   ports the instance already holds. In `assignWithin`, a bound port that is one of
   `Own` is treated as usable (it's the instance's own service, not a squatter), so
   re-leasing a running checkout is idempotent. A genuinely external squatter (a
   bound port that is *not* owned) still relocates. The daemon builds `Own` from the
   instance's stored berths.
2. **Read-only `get` op + `hm ports`.** A new `get` op returns the instance's stored
   lease without allocating, probing, or writing state. `hm ports` uses `get` when a
   lease exists and only falls back to `lease` on first use — so showing your ports
   never mutates them.

**Residual tradeoff:** if an external process squats *exactly* one of an instance's
recorded ports while it is stopped, the ownership rule hands that port back anyway
(then `hm up` → Tilt surfaces the bind error). Rare, and the alternative — moving
your own running services — is worse and common. Precise resolution needs liveness
tracking (roadmap). This also bounds the `doctor` caveat in [D8]: `doctor` still
*reports* bound leased ports, but the allocator no longer *acts* on them.

## 2026-06-18 — CLI

### D9 — `hm down` keeps the lease

SPEC §6 says `hm down` should "mark inactive". MVP has no liveness tracking (that
is roadmap, SPEC §10), and releasing the lease would discard the instance's stable
block — defeating the whole point of stable per-worktree ports. So `hm down` runs
`tilt down` with the HM_* env and **keeps** the lease; freeing it is explicit via
`hm release`. "Inactive" marking arrives with heartbeat liveness later.

## 2026-06-18 — daemon / ipc

### D7 — Lease engine lives in `internal/daemon`, not `cmd/`

`internal/ipc` is pure transport (message types + client + server) and imports no
other internal package. The lease logic (wiring alloc+state+config under one
mutex, implementing `ipc.Handler`) lives in `internal/daemon` so it is importable
by **both** the `harbormasterd` binary and the CLI's `hm daemon` subcommand —
package `main` can't be imported. Auto-start (next increment) will re-exec the CLI
as `hm daemon` rather than requiring a separate `harbormasterd` on PATH.

### D8 — `doctor` squatters = leased ports currently bound (MVP)

The daemon can't distinguish "a lease's own Tilt is using its port" from "an
external process squats a leased port" without liveness tracking (roadmap). So MVP
`doctor` reports any leased port that fails a bind-probe under `squatters`, and the
docs/CLI phrase it as "in use" — expected for an active checkout, actionable only
for one believed idle. Also renamed the reply's instance count to `leases` to avoid
colliding with `list`'s `instances` array.

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

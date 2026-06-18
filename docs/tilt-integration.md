# Tilt integration

harbormaster feeds host ports to Tilt; it never generates or supervises Tiltfiles.
There are two layers, lowest-friction first. Target Tilt **0.36+**.

## Layer 1 — environment variables (primary, zero-dependency)

`hm up` (and `hm ports --env`) export a stable env for the current checkout:

```sh
HM_TILT_PORT=20000
HM_PORT_WEB=20001
HM_PORT_API=20002
HM_PORT_POSTGRES=20003
```

- `HM_TILT_PORT` — the Tilt web UI port (`tilt up --port`).
- `HM_PORT_<SERVICE>` — one per service, uppercased.

A retrofitted Tiltfile reads them with a tiny helper and keeps its old hardcoded
port as the default, so it still works when harbormaster is **not** installed:

```python
# --- harbormaster: read assigned host ports, fall back to legacy defaults ---
def hm_port(name, default):
    return int(os.getenv('HM_PORT_' + name.upper(), str(default)))

WEB      = hm_port('web', 3000)
API      = hm_port('api', 4000)
POSTGRES = hm_port('postgres', 5432)
```

Then use those variables wherever a port was hardcoded — `local_resource` serve
commands, `port_forward`, `docker_compose` published ports, etc.

## Layer 2 — the `hm up` wrapper (handles Tilt's own port)

Developers run `hm up` instead of `tilt up`. It resolves the lease for `$PWD`,
exports `HM_*`, and execs:

```sh
tilt up --port "$HM_TILT_PORT" <any extra tilt args after -->
```

So `hm up -- --stream` becomes `tilt up --port 20000 --stream`. `hm down` runs
`tilt down` and marks the instance inactive.

## Retrofit procedure (existing Tiltfile)

1. Inventory every hardcoded host port in the Tiltfile (UI port, `local_resource`
   ports, `port_forward` locals, compose `ports:` host sides).
2. Add the `hm_port` helper near the top (and `load('ext://...')` / `os` import if
   not present — `os` is available in Tiltfiles by default).
3. Replace each hardcoded port with `hm_port('<service>', <old_default>)`, keeping
   the old number as the default.
4. Run `hm init` to declare those services (or hand-write `harbormaster.toml`) so
   each gets a stable offset.
5. Launch with `hm up`. Verify the Tilt UI comes up on `HM_TILT_PORT` and each
   service is reachable on its `HM_PORT_*`.
6. Commit `harbormaster.toml` and the Tiltfile changes.

## Multiple worktrees at once

Because the allocation key is `(worktree_path, service)`, two worktrees of the same
repo get **different** blocks, so `hm up` in each runs both stacks simultaneously
with zero port contention — no manual edits per checkout.

## Roadmap (not MVP)

A loadable Starlark extension `ext://harbormaster` exposing `hm = harbormaster()` /
`hm.port('web')` for projects that prefer in-Tiltfile resolution over env vars. See
SPEC §5c / §10.

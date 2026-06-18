# Tilt CLI reference (for retrofitting)

[Tilt](https://tilt.dev) (target **0.36+**) runs your local dev stack from a
`Tiltfile`. harbormaster feeds it host ports; it does not replace it. This is the
slice of Tilt you need to retrofit a project.

## Running

- `tilt up [--port N] [--host HOST] [--stream] [resource...] [-- <tiltfile args>]`
  Start Tilt and its web UI. `--port` sets the UI port (default `10350`) — this is
  exactly what `hm up` overrides with `HM_TILT_PORT`. Naming resources limits what
  runs. `--stream` mirrors logs to stdout.
- `tilt down [-- <tiltfile args>]` — stop and delete what the Tiltfile created.
- `tilt ci` — run to completion once; non-zero exit on failure (use in CI).

## Observing / driving

- `tilt logs [-f] [resource...]` — print/stream logs.
- `tilt get uiresources` / `tilt get session` — current state as YAML/JSON.
- `tilt describe <resource>` — details for one resource.
- `tilt trigger <resource>` — force an update.
- `tilt args [-- <args>]` — change Tiltfile config args without restarting.
- `tilt version`, `tilt doctor`.

## Where host ports live in a Tiltfile

1. **Tilt UI** — `tilt up --port` (handled by `hm up` via `HM_TILT_PORT`; no
   Tiltfile change needed).
2. **`local_resource`** servers — ports baked into `serve_cmd`, e.g.
   `serve_cmd='PORT=3000 npm start'`.
3. **`k8s_resource(..., port_forwards=[...])`** — `'LOCAL:CONTAINER'` pairs.
4. **`docker_compose`** published ports — `ports: ["HOST:CONTAINER"]` in the
   compose file.

## Tiltfile building blocks

- `os.getenv('NAME', default)` — read an env var (how harbormaster ports arrive).
- `local_resource(name, serve_cmd=..., cmd=..., deps=[...], resource_deps=[...])`
- `docker_compose('docker-compose.yml')`
- `k8s_yaml(...)` + `k8s_resource(name, port_forwards=['3000:3000'])`
- `config.define_string('x'); cfg = config.parse()` — typed Tiltfile args.

See [tiltfile-retrofit.md](tiltfile-retrofit.md) for the exact edits.

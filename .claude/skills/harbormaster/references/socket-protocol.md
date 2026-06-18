# Daemon socket protocol (summary)

The CLI talks to `harbormasterd` over a **Unix domain socket** using **NDJSON**:
one JSON request line, one JSON reply line, then the connection closes. You rarely
call this directly — the `hm` CLI wraps it — but it's here for tooling. The fuller
version lives in `docs/socket-protocol.md` in the repo.

- **Socket:** `${XDG_RUNTIME_DIR}/harbormaster/hm.sock`, else
  `~/.local/share/harbormaster/hm.sock`.
- Every reply has `"ok": true|false`; failures carry `"error"`.

## Ops

```jsonc
{"op":"ping"}
// → {"ok":true,"version":"…"}

{"op":"lease","instance":"/abs/path","project":"p","label":"feat","services":["web","api"]}
// → {"ok":true,"tilt":20020,"ports":{"web":20021,"api":20022},"block":[20020,20039],"warnings":[]}

{"op":"list"}
// → {"ok":true,"instances":[{instance,project,label,block,berths,createdAt,lastSeenAt}]}

{"op":"release","instance":"…"}
// → {"ok":true,"released":true}

{"op":"prune"}
// → {"ok":true,"reclaimed":["…"]}

{"op":"doctor"}
// → {"ok":true,"leases":N,"headroom":N,"squatters":[{instance,service,port}]}
```

`lease` is idempotent per instance: the block base is stable once assigned. The
daemon places `tilt` at block offset 0 and services at offsets 1..N **in request
order**, so `services` should be sent in the order they appear in
`harbormaster.toml` (the CLI sorts by configured `offset`). `ping` doubles as the
liveness check the CLI uses to decide whether to auto-start the daemon.

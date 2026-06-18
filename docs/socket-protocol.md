# Socket protocol

The CLI talks to `harbormasterd` over a **Unix domain socket** using **NDJSON**:
one JSON object per line for the request, one JSON object per line for the reply.
This refines SPEC §8 with exact field names; the implementation in `internal/ipc`
follows this doc.

- **Socket path:** `${XDG_RUNTIME_DIR}/harbormaster/hm.sock`, else
  `~/.local/share/harbormaster/hm.sock`.
- **Framing:** newline-delimited. The client writes one request line, the server
  writes one reply line, then the connection closes (one request per connection).
- **Encoding:** UTF-8 JSON, no embedded newlines within a message.

## Common reply shape

Every reply has a boolean `ok`. On failure:

```json
{"ok": false, "error": "human-readable message"}
```

On success `ok` is `true` plus op-specific fields below.

## Ops

### `ping` — health check (used by CLI auto-start)

```json
→ {"op": "ping"}
← {"ok": true, "version": "1.2.3"}
```

### `lease` — resolve this instance's ports

```json
→ {"op": "lease",
   "instance": "/Users/av/dev-vd/groundtruth/feat-x",
   "project": "groundtruth",
   "label": "feat-x",
   "services": ["web", "api", "postgres"]}
← {"ok": true,
   "tilt": 20020,
   "ports": {"web": 20021, "api": 20022, "postgres": 20023},
   "block": [20020, 20039],
   "warnings": []}
```

- `instance` (required) — absolute checkout path; the allocation key.
- `project`, `label` — metadata for display; not part of the key.
- `services` — ordered service names. The daemon assigns `tilt` at block offset 0
  and each service at offset `1 + index`. The CLI orders `services` by their
  configured `offset` in `harbormaster.toml`, so the contiguous-offset case (the
  SPEC worked example) reproduces the configured ports. **Non-contiguous explicit
  offsets and per-worktree pins are roadmap (SPEC §4.4), not MVP.**
- `warnings` — non-fatal notes, e.g. a previously-leased port now squatted and
  relocated.

`lease` is idempotent for a given instance: the block base is stable across calls
once assigned.

### `list` — every instance

```json
→ {"op": "list"}
← {"ok": true, "instances": [
     {"instance": "/Users/av/dev-vd/groundtruth/main",
      "project": "groundtruth", "label": "main",
      "block": [20000, 20019],
      "berths": {"tilt": 20000, "web": 20001, "api": 20002},
      "createdAt": "2026-06-18T10:00:00Z",
      "lastSeenAt": "2026-06-18T11:30:00Z"}
   ]}
```

### `release` — free one instance's lease

```json
→ {"op": "release", "instance": "/Users/av/dev-vd/groundtruth/feat-x"}
← {"ok": true, "released": true}
```

`released` is `false` if the instance held no lease.

### `prune` — reclaim leases whose worktree dir is gone

```json
→ {"op": "prune"}
← {"ok": true, "reclaimed": ["/Users/av/dev-vd/groundtruth/old-branch"]}
```

### `doctor` — daemon / pool health

```json
→ {"op": "doctor"}
← {"ok": true,
   "leases": 3,
   "headroom": 597,
   "squatters": [{"instance": "...", "service": "web", "port": 20001}]}
```

- `leases` — number of leased instances.
- `headroom` — number of free blocks remaining in the pool.
- `squatters` — leased ports that fail a bind-probe, i.e. currently in use. Note:
  in the MVP this includes a lease's **own** Tilt/services while that instance is
  running (harbormaster does not track liveness yet — that is roadmap, SPEC §10).
  So a squatter entry means "this leased port is occupied", which is expected for
  an active checkout and only actionable for one you believe is idle.

### `ping` — also used to detect a live daemon

The CLI auto-start logic dials the socket and sends `ping`; a successful `ok:true`
reply means a daemon is already running, so it does not start another.

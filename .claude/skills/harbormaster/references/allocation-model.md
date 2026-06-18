# Allocation model

How harbormaster assigns ports. Authoritative design: `SPEC.md` §4.

## Pool, blocks, berths

- A machine-wide **pool**, default `[20000, 32000)`, split into fixed **blocks**
  (default 20 ports). Configurable in `~/.config/harbormaster/config.toml`
  (`pool_start`, `pool_end`, `block_size`, `reserved`).
- Each **instance** (checkout/worktree) gets one contiguous block. The base is
  assigned on first lease and **stable** until released/pruned.
- Within a block: **offset 0 = the Tilt UI port**; services take offsets 1, 2, 3, …
  in declared order. So `tilt=base`, `web=base+1`, `api=base+2`, …

## Worktree-aware (the whole point)

The allocation key is `(instance_path, service)`, not `(project, service)`. Two
worktrees of one repo get **different blocks**, so both `tilt up` at once with zero
contention:

```
groundtruth @ .../main      block 20000–20019   tilt 20000  web 20001  api 20002
groundtruth @ .../feat-x    block 20020–20039   tilt 20020  web 20021  api 20022
```

## Truthful assignment

Before handing out a port the daemon **bind-probes** `127.0.0.1:port` and skips any
**reserved** port. If a nominal berth is taken (external process) or reserved, the
berth is **relocated** to another free offset in the block and a warning is emitted.
New instances take the **lowest free block**; a freed block is reused before the
pool extends.

## Determinism

Same set of instances + same config ⇒ same assignment. Blocks never overlap
between live instances.

## Config knobs (`config.toml`)

```toml
pool_start = 20000
pool_end   = 32000
block_size = 20
reserved   = [5432, 6379]   # never hand these out
```

## Per-project topology (`harbormaster.toml`, committed)

```toml
name = "groundtruth"

[services]
web      = { offset = 1, default = 3000 }
api      = { offset = 2, default = 4000 }
postgres = { offset = 3, default = 5432 }
```

`offset` is the slot within the block; `default` is the legacy port used when
harbormaster is absent. Committing this means teammates share the topology; each
machine's daemon assigns its own locally-free blocks.

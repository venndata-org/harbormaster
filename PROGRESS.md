# PROGRESS

Build checklist for the harbormaster MVP. Mirrors [`SPEC.md`](./SPEC.md) §10. Tick
an item only when the verification gate is green: `go build ./...`, `go vet ./...`,
and `go test ./...` all pass.

## Phase 0 — scaffold

- [x] Compiling Go stubs (CLI router + `--version`, daemon entrypoint)
- [x] CLAUDE.md, Makefile, harbormaster.example.toml
- [x] docs/{architecture,socket-protocol,tilt-integration}.md
- [x] PROGRESS.md + DECISIONS.md
- [x] First commit + public GitHub repo pushed

## MVP (SPEC §10)

- [x] `internal/config` — global `config.toml` + per-project `harbormaster.toml`
- [x] `internal/gitident` — git-derived project / instance / label (worktree-aware)
- [x] `internal/alloc` — deterministic block allocator + bind-probe + reserved ports
      **(heavily unit-tested)**
- [x] `internal/state` — atomic `state.json` load/save
- [x] `internal/ipc` + `harbormasterd` — Unix socket, NDJSON ops
      (lease / list / release / prune / doctor)
- [x] CLI auto-starts the daemon
- [x] Wire CLI: `ports`, `up`, `down`, `ls`, `release`, `prune`, `doctor`, `init`
- [x] Env-var Tilt integration + `hm up` wrapper
- [ ] Claude Code skill (SKILL.md + references: CLI, Tilt CLI, retrofit, allocation,
      protocol)
- [x] e2e smoke test: two fake instances of one project → assert non-overlapping
      blocks & ports → release both (internal/daemon TestE2E_SmokeTwoInstances)

## Blocked

_(none)_

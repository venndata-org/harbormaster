# Seed prompt — put a project on harbormaster

A ready-to-paste task prompt. Hand it to a coding agent (Claude Code) **inside the
repo you want to retrofit**. It drives the full job: reserve this project's ports
with harbormaster, check for conflicts with other reserved/in-use ports, and update
the project's infra so nothing collides. Copy everything in the box below.

---

> **Task: put this project on harbormaster (conflict-free local Tilt ports).**
>
> Use the **harbormaster** skill — read `.claude/skills/harbormaster/SKILL.md` and
> its `references/` (especially `tiltfile-retrofit.md` and `cli-commands.md`).
> harbormaster leases each git worktree a stable, conflict-free block of host ports
> for Tilt; the CLI is `hm`. Work in the current repo (`$PWD`). Make reasonable
> engineering calls; don't ask me unless something is genuinely ambiguous.
>
> **0. Preflight.** Confirm `hm --version` works. If not, install harbormaster per
> the skill (`make install` from the harbormaster repo; ensure `~/go/bin` is on
> `PATH`). harbormaster's daemon auto-starts — don't start it by hand.
>
> **1. Inventory this project's host ports.** Grep the repo for hardcoded host
> ports: `Tiltfile`, `docker-compose*.yml`, `.env*`, k8s manifests, `Procfile`,
> `package.json` scripts, `Makefile`, app configs. Look for `:3000`, `port 4000`,
> `ports: ["5432:5432"]`, `--port`, `localhost:<port>`, `0.0.0.0:<port>`. Produce a
> list of `service -> current default port`. Note the Tilt UI port if pinned
> (default 10350).
>
> **2. Check for conflicts (other reserved / in-use ports).**
> - `hm ls` — every port block already leased to other projects/worktrees on this
>   machine. Yours must not overlap (harbormaster guarantees this, but confirm).
> - `hm doctor` — pool range, free blocks (headroom), leased ports currently in use.
> - `~/.config/harbormaster/config.toml` `reserved = [...]` — ports harbormaster
>   never hands out; your *defaults* shouldn't silently depend on them either.
> - Host reality: `lsof -iTCP -sTCP:LISTEN -nP | sort` (or
>   `netstat -an | grep LISTEN`). Flag anything bound inside harbormaster's pool
>   `[20000,32000)` or on your service defaults. (harbormaster bind-probes and
>   routes around live squatters automatically — but surface anything surprising.)
>
> **3. Reserve this project's ports.** Run
> `hm init --service <name>:<default> ...` for each service from step 1 (offsets are
> assigned in declared order; offset 0 is the Tilt UI). This writes
> `harbormaster.toml` at the repo root. Then `hm ports` (and `hm ports --json`) to
> see the assigned block and per-service ports.
>
> **4. Update the infra to read the assigned ports** (use `HM_PORT_*` when present,
> fall back to old defaults so the project still works without harbormaster):
> - **Tiltfile** — add the helper and replace every hardcoded host port:
>   ```python
>   def hm_port(name, default):
>       return int(os.getenv('HM_PORT_' + name.upper(), str(default)))
>   ```
>   e.g. `local_resource(serve_cmd='PORT=%d ...' % hm_port('web', 3000))`,
>   `k8s_resource(port_forwards=['%d:8080' % hm_port('api', 4000)])`. The Tilt UI
>   port needs no change — `hm up` passes `--port $HM_TILT_PORT`.
> - **docker-compose** — env-interpolate published ports with a default:
>   `ports: ["${HM_PORT_POSTGRES:-5432}:5432"]`. `hm up` exports `HM_PORT_POSTGRES`.
> - **Other config / scripts / .env** — point them at `HM_PORT_*` / `HM_TILT_PORT`
>   with the same fallback pattern.
> - Service names in `hm_port('NAME', …)` must match the names passed to `hm init`
>   (case-insensitive; the env var is `HM_PORT_<UPPER>`).
> - Follow `references/tiltfile-retrofit.md` for exact before/after edits.
>
> **5. Verify.** `hm up` (needs Tilt 0.36+): confirm the Tilt UI is on
> `HM_TILT_PORT` and each service is reachable on its `HM_PORT_*`. Then prove
> no-conflict: `git worktree add ../<branch> -b <branch>`, `cd` there, `hm up` again
> — both stacks must run at once on different blocks. `hm ls` shows non-overlapping
> blocks for both worktrees.
>
> **6. Commit.** Commit `harbormaster.toml` and the Tiltfile/compose/config changes.
> The defaults keep everything working for anyone without harbormaster installed.
>
> **Deliverable:** `harbormaster.toml` committed, infra reading `HM_PORT_*`, `hm up`
> verified across two worktrees, and a short note of any ports that collided and how
> you resolved them.

---

(Generated from the harbormaster skill. Keep this prompt in sync with the CLI
surface in `cli-commands.md` and the retrofit steps in `tiltfile-retrofit.md`.)

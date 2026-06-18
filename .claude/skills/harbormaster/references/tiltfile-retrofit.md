# Retrofit a Tiltfile to read harbormaster ports

Goal: replace every hardcoded host port with `hm_port('<service>', <old_default>)`
so the Tiltfile uses **assigned** ports when harbormaster is present and its **old**
ports otherwise. Then launch with `hm up`.

## Step 1 — inventory hardcoded host ports

Search the Tiltfile (and any compose file) for host ports:
- the Tilt UI port (if pinned),
- `local_resource` `serve_cmd` ports,
- `port_forward` / `port_forwards` local ports,
- `docker_compose` published `ports:` host sides.

## Step 2 — add the helper

Near the top of the Tiltfile:
```python
def hm_port(name, default):
    return int(os.getenv('HM_PORT_' + name.upper(), str(default)))
```
(`os` is available in Tiltfiles by default.)

## Step 3 — declare the services

```sh
hm init --service web:3000 --service api:4000 --service postgres:5432
```
This writes `harbormaster.toml`. Service names must match the `hm_port('NAME', …)`
names (case-insensitive — the env var is `HM_PORT_<UPPER>`).

## Step 4 — replace the ports

**local_resource serve_cmd:**
```python
# before
local_resource('web', serve_cmd='PORT=3000 npm start')
# after
WEB = hm_port('web', 3000)
local_resource('web', serve_cmd='PORT=%d npm start' % WEB)
```

**k8s port_forwards (`LOCAL:CONTAINER`):**
```python
# before
k8s_resource('api', port_forwards=['4000:4000'])
# after
API = hm_port('api', 4000)
k8s_resource('api', port_forwards=['%d:4000' % API])
```

**docker_compose published ports** — parametrize the compose file with env (Tilt
and docker-compose interpolate `${VAR}`); harbormaster sets it:
```yaml
# docker-compose.yml
services:
  postgres:
    ports: ["${HM_PORT_POSTGRES:-5432}:5432"]
```
`hm up` exports `HM_PORT_POSTGRES`, so the published host port follows the lease;
without harbormaster it falls back to 5432.

**Tilt UI port** — no Tiltfile change needed; `hm up` runs
`tilt up --port $HM_TILT_PORT`.

## Step 5 — run and verify

```sh
hm up
hm ports        # confirm the assignments
```
Open the Tilt UI on `HM_TILT_PORT`; confirm each service is reachable on its
`HM_PORT_*`. In a second worktree, `hm up` again — both stacks run at once on
different blocks.

## Step 6 — commit

Commit `harbormaster.toml` and the Tiltfile (and compose) changes. The defaults
keep the Tiltfile working for anyone who hasn't installed harbormaster.

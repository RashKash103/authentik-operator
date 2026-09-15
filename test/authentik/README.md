# Local authentik test instance

A disposable authentik stack for developing and testing the operator. The same
compose file backs both `just authentik-up` on a laptop and the E2E job in CI.

The point of this directory is that authentik comes up **fully provisioned and
API-authenticable without anyone touching a browser**. A stock authentik boots
into an out-of-box-experience flow that demands a human set the admin password;
that is fine for a real deployment and useless for CI. Setting
`AUTHENTIK_BOOTSTRAP_TOKEN`, `AUTHENTIK_BOOTSTRAP_PASSWORD` and
`AUTHENTIK_BOOTSTRAP_EMAIL` makes authentik apply its `system/bootstrap.yaml`
blueprint on first start, which creates the `akadmin` superuser and a
**non-expiring API token whose key is exactly `AUTHENTIK_BOOTSTRAP_TOKEN`**. The
E2E suite can therefore hard-code a token it already knows.

## Contents

| File | Purpose |
| --- | --- |
| `docker-compose.yaml` | PostgreSQL + authentik `server` + authentik `worker` |
| `up.sh` | Starts the stack and blocks until an authenticated API call succeeds |
| `down.sh` | Stops the stack and removes its volumes |
| `lib.sh` | Config resolution and helpers shared by the two scripts |
| `.env.example` | Every variable, documented, with throwaway defaults |

## Quick start

```shell
just authentik-up      # or: ./test/authentik/up.sh
just test-e2e
just authentik-down    # or: ./test/authentik/down.sh
```

`up.sh` prints the resolved URL, the admin credentials and the API token when it
finishes. Nothing else is needed: the defaults in `.env.example` are baked into
`lib.sh` and `docker-compose.yaml`, so the scripts work with no `.env` at all.

### How long it takes

Measured on a developer workstation with the images already pulled, the gate
clears in **45-50 seconds** from `docker compose up` to the first successful
authenticated call (2026.8.2: 48s; 2026.2.7: 45s), broken down roughly as:

| Window | What is happening |
| --- | --- |
| ~0-30s | Django runs the full migration set on an empty database |
| ~30-40s | API starts serving, but still answers **403** - the token does not exist yet |
| ~40-50s | worker imports the blueprints, bootstrap token is created, probe returns 200 |

That 403 window is precisely why readiness is gated on an authenticated call.

Every run is a cold start, because `down.sh` deliberately wipes the volumes.
The first run on a machine also pays for the image pull (~1GB). CI runners are
slower and noisier than a workstation, so `AUTHENTIK_READY_TIMEOUT` defaults to
**600 seconds** - roughly 12x the observed time. That headroom is intentional:
the cost of a too-generous timeout is a slow failure, the cost of a too-tight
one is a flaky suite.

## How readiness is decided

This is the part worth understanding, because getting it wrong produces flaky
E2E runs that are painful to diagnose.

`up.sh` polls:

```
GET ${AUTHENTIK_URL}/api/v3/core/users/me/
Authorization: Bearer ${AUTHENTIK_BOOTSTRAP_TOKEN}
```

and only declares success on **HTTP 200 with a user payload in the body**.

It deliberately does *not* use:

* a TCP connect or port check - the listener binds long before Django is serving;
* an unauthenticated health endpoint such as `/-/health/live/` - that answers
  while migrations are still running and, critically, before the bootstrap
  blueprint has created the token, so a suite gated on it races authentik and
  fails intermittently.

Only an authenticated call proves the three things the suite actually needs:
the HTTP stack serves, the database is migrated, and the bootstrap token exists.

Knobs: `AUTHENTIK_READY_TIMEOUT` (default `600` seconds) and
`AUTHENTIK_READY_INTERVAL` (default `3`). On expiry `up.sh` prints the last HTTP
status, a list of likely causes, `docker compose ps`, and the last 60 log lines
from each container, then exits non-zero **leaving the stack running** so you
can inspect it.

## Pointing the operator at it

`up.sh` prints these; they are also the values CI exports:

```shell
export AUTHENTIK_URL=http://localhost:9000
export AUTHENTIK_TOKEN=authentik-operator-e2e-bootstrap-token
```

Verify by hand:

```shell
curl -sS -H "Authorization: Bearer $AUTHENTIK_TOKEN" \
  "$AUTHENTIK_URL/api/v3/core/users/me/" | jq .
```

### From inside a kind cluster

The server port is published on all host interfaces, so a pod can reach it via
the host gateway rather than `localhost` (which inside a pod means the pod):

* Docker Desktop (macOS / Windows): `http://host.docker.internal:9000`
* Linux: the docker bridge address, usually `http://172.17.0.1:9000` - confirm
  with `ip -4 addr show docker0`

Put whichever applies in the `AuthentikConnection` (or equivalent) the operator
reads, not `localhost`.

## Switching authentik versions

The supported versions and their pinned tags are in
[`supported-versions.yaml`](../../supported-versions.yaml) at the repo root.
That file is the single source of truth; this directory just consumes it.

```shell
AUTHENTIK_TAG=2026.5.7 ./test/authentik/up.sh
AUTHENTIK_TAG=2026.2.7 ./test/authentik/up.sh
```

Currently pinned:

| Series | Tag |
| --- | --- |
| 2026.8 | `2026.8.2` |
| 2026.5 | `2026.5.7` |
| 2026.2 | `2026.2.7` |

`AUTHENTIK_IMAGE` overrides the repository (default
`ghcr.io/goauthentik/server`) if you need a local build or a mirror.

**Always `./down.sh` before switching versions.** authentik migrations are
forward-only; pointing an older image at a database an newer image has migrated
fails in confusing ways. `down.sh` removes the volume, so the next `up.sh`
starts from an empty database.

## Admin UI, for debugging

Open <http://localhost:9000/if/admin/> and log in as:

* **username** `akadmin` (the email, `akadmin@e2e.localhost`, also works)
* **password** the value of `AUTHENTIK_BOOTSTRAP_PASSWORD`, by default
  `authentik-operator-e2e-admin-password`

Useful while debugging a failing E2E test: **Directory -> Tokens** lists the
bootstrap token, **Events -> Logs** shows every API write the operator made, and
**Applications / Providers / Flows** show the objects it created.

Logs:

```shell
cd test/authentik
docker compose logs -f server
docker compose logs -f worker
docker compose logs -f postgresql
```

Keep the database across a restart while poking at it:

```shell
./down.sh --keep-volumes
```

## Configuration

Copy `.env.example` to `.env` and edit. `.env` is gitignored.

```shell
cp test/authentik/.env.example test/authentik/.env
```

Precedence, highest first:

1. variables exported in the shell
2. `test/authentik/.env`
3. defaults in `lib.sh` / `docker-compose.yaml`

That ordering is what lets CI drive the matrix with `AUTHENTIK_TAG=... ./up.sh`
while a developer keeps a `.env` with, say, a different port.

> **These credentials are throwaway.** They are committed on purpose so CI needs
> no secret store. Never reuse them for an authentik instance holding real users
> or reachable from anywhere but your machine or the CI runner.

## Notes on the topology

Based on the upstream stack published at <https://goauthentik.io/compose.yml>,
with deliberate differences:

* **Named volumes** instead of upstream's `./data`, `./certs` and
  `./custom-templates` bind mounts, so `down -v` wipes every trace of a run and
  nothing is left in the working tree.
* **No `/var/run/docker.sock` mount on the worker.** Upstream mounts it for the
  embedded outpost's Docker controller, which the E2E suite never exercises;
  handing a container the Docker socket in CI is an unnecessary privilege.
* **Bootstrap environment variables**, as described above. Upstream does not set
  them because a real install wants a human to choose the admin password.
* **No Redis.** authentik removed its Redis dependency in **2025.10** - cache,
  channel layer and task broker all moved to PostgreSQL, and there is no
  `redis:` key left in `authentik/lib/default.yml` for any version this operator
  supports (2026.2 and newer). Upstream's current compose file has no Redis
  service either. A `redis` service is still defined here under the
  `legacy-redis` compose profile, **not started by default**, for anyone
  pointing this stack at a pre-2025.10 image:

  ```shell
  cd test/authentik
  AUTHENTIK_REDIS__HOST=redis docker compose --profile legacy-redis up -d
  ```

`server` and `worker` share one image and one environment block (a YAML anchor
keeps them in sync) and differ only in their `command`, matching upstream.

---
name: bump-authentik-version
description: Update the operator to support a newer authentik release - bump the generated API client, retarget the version gate and CI matrix, reconcile API drift, and verify against a real instance. Use when a new authentik version is out, when asked to support a newer authentik, or when the E2E matrix fails against a newer release.
---

# Bump the supported authentik version

One operator release targets **one** authentik series. Supporting a newer
authentik is therefore not a config edit — it is a retarget of the whole
project, ending in a new versioned release.

Read `README.md` → "Version policy" before starting. If you are trying to
support *two* authentik versions at once, this skill is the wrong tool: see the
Epiq ticket on multi-version support, because the generated client makes that a
design change rather than a bump.

## Why this is more than editing a YAML file

`goauthentik.io/api/v3` is generated from a single authentik release and
**enforces that release's required properties when decoding**. A client built
for 2026.8 marks fields as required that do not exist in 2026.5, so every list
call against the older version fails outright. The client version and the
authentik version move together, or nothing works.

## Steps

### 1. Establish the target

Find the authentik release and the matching client:

```sh
# Real authentik image tags
TOKEN=$(curl -s "https://ghcr.io/token?scope=repository:goauthentik/server:pull&service=ghcr.io" | jq -r .token)
curl -s -H "Authorization: Bearer $TOKEN" "https://ghcr.io/v2/goauthentik/server/tags/list?n=2000" \
  | jq -r '.tags[]' | grep -E '^20[0-9]{2}\.[0-9]+\.[0-9]+$' | sort -V | tail -10

# Client versions. The client's version encodes the series: v3.2026080.x is 2026.8
go list -m -versions goauthentik.io/api/v3 | tr ' ' '\n' | tail -15
```

**Verify the tag exists before using it.** Guessing the series has already gone
wrong here once — the real series are `2026.2`, `2026.5`, `2026.8`, not every
even month.

Pick the newest **stable** patch tag, not an `-rc`.

**If the newest series is the one already pinned**, there is no retarget to do.
What is left is a patch bump: a newer client patch and a newer tested image
within the same series. Do steps 2, 5 and 6, skip the version-metadata retarget
in step 4 beyond the `image:` field, and add no compatibility-matrix row — the
series has not changed. That is a patch release of the operator, not a minor
one.

**If the pinned series has newer patches, or newer series have appeared, work
forward one series at a time and release after each.**

A client is generated from one series and cannot decode another, so an operator
release has to exist for every series a user might be running. Jumping straight
to the newest leaves anyone on an intermediate series with nothing to install.

Each series is released at its **final patch**, not the patch that happened to
be pinned when the series was current. So if the operator sits on `2026.6.2`,
`2026.6.3` shipped afterwards, and the target is `2026.8.1`, that is two
releases: first `2026.6.3`, then `2026.8.1`. The `2026.6.3` release is what
someone still on that series installs, and without it the docs table would point
them at an operator built for `2026.6.2`.

Enumerate what that means before starting:

```sh
# Newest patch per series, from the pinned image forward. Each line is one
# release, in the order they must be done.
PINNED=$(python3 -c "import yaml;print(yaml.safe_load(open('supported-versions.yaml'))['supported'][-1]['image'].rsplit(':',1)[-1])")
TOKEN=$(curl -s "https://ghcr.io/token?scope=repository:goauthentik/server:pull&service=ghcr.io" | jq -r .token)
curl -s -H "Authorization: Bearer $TOKEN" "https://ghcr.io/v2/goauthentik/server/tags/list?n=2000" \
  | jq -r '.tags[]' | grep -E '^20[0-9]{2}\.[0-9]+\.[0-9]+$' | sort -V \
  | awk -F. -v pin="$PINNED" '
      { k=$1"."$2; last[k]=$0; if (!seen[k]++) order[++n]=k }
      END { split(pin,p,"."); for (i=1;i<=n;i++) { split(order[i],q,".");
              if (q[1]>p[1] || (q[1]==p[1] && q[2]>=p[2])) print last[order[i]] } }'
```

The first line is the pinned series at its newest patch — a release of its own
if that patch is newer than the one currently tested. Lines after it are the
series to move through, in order.

Only the newest operator release per series is shown in the docs lookup, so a
series that gained several operator releases lists just the last one.

### 1b. See what will break before you touch anything

```sh
just check-api-drift --candidate <new-series>
```

This compares authentik's published schema against the pinned series and names
the fields whose `required` status changed on schemas the operator decodes. It
is the same class of break that pins the operator to one series at a time, so
read it before starting: it tells you which adapters step 3 will touch.

An empty report does not mean no work. It means no *decode-breaking* shape
change; renamed Go symbols in the generated client are a separate matter and
still show up as compile errors in step 2.

### 2. Bump the client

```sh
go get goauthentik.io/api/v3@<new-client-version>
go mod tidy
go build ./... 2>&1 | head -40
```

Expect breakage. The generated client renames things between releases — an
older one spells the service fields `FlowsApi`, a newer one `FlowsAPI`. Fix the
call sites; do not pin around the problem.

### 3. Reconcile API drift

This is the real work. For each compile error and each field the operator maps:

- Read the model in `$(go env GOMODCACHE)/goauthentik.io/api/v3@<version>/` —
  **never guess a signature or an enum value**.
- Check whether fields the CRDs expose still exist, changed type, or became
  required.
- Check whether new fields are worth exposing. Adding them is optional; breaking
  on removed ones is not.
- Enum values change. Re-extract them:

  ```sh
  M=$(go env GOMODCACHE)/goauthentik.io/api/v3@<version>
  grep -oE '= "[^"]+"' $M/model_<enum>_enum.go | sed 's/= //' | tr '\n' ' '
  ```

  If an enum changed, update the `+kubebuilder:validation:Enum` marker **and**
  remember that values containing `:` need individual quoting, and that a marker
  on a slice field must move to a named element type.

### 4. Retarget the version metadata

`supported-versions.yaml` is the single source of truth. Update `supported`,
`minimum` and `maximum` to the new series, then:

```sh
just sync-versions   # rewrites the README table and bounds
```

Update the Go gate constants in `internal/authentik/version.go` (`MinimumVersion`,
`MaximumVersion`) to match — `TestSupportedVersionConstantsMatchYAML` fails if
they drift.

Add an entry to `releases:` in `supported-versions.yaml` — `operator`, `tag`,
`authentik` and a one-line `notes` — then re-run `just sync-versions`. That
renders both tables in `README.md` and `docs/operations/supported-versions.md`:
the compatibility matrix (operator to authentik) and the "Running an older
authentik?" lookup (authentik to the release to install). Do not hand-edit
either; they carry generated markers.

Do not delete old entries. Users on an older authentik stay on an older operator
release, and those tables are how they find it.

### 5. Regenerate everything

```sh
just manifests generate
just docs-gen
just verify
```

`just verify` must pass on a clean tree. It also syncs the chart's CRDs and RBAC.

### 6. Verify against a real instance

A compile is not evidence. Run the operator against the new authentik:

```sh
just authentik-up
curl -s -H "Authorization: Bearer authentik-operator-e2e-bootstrap-token" \
  http://localhost:9000/api/v3/admin/version/ | jq -r .version_current
just test-integration                 # client layer against the live instance
```

**Check that version line every time.** The tag was written in five places, and
the two that decide what actually starts locally — a shell default in
`test/authentik/lib.sh` and a compose interpolation default in
`docker-compose.yaml` — were not derived from `supported-versions.yaml` until
this was found. CI reads the tag out of the YAML and tested the new patch while
`just authentik-up` still started the old one. `just sync-versions` now owns
both, and `--check` fails when they drift, but confirm the running version
rather than trusting the file: a stale container or a `test/authentik/.env` will
also quietly win.

Then the full end-to-end suite. If `kind` is not installed,
`go install sigs.k8s.io/kind@latest` into `bin/`:

```sh
./bin/kind create cluster --name ak-bump --wait 120s
./bin/kustomize build config/crd | kubectl --context kind-ak-bump apply -f -
go build -o bin/manager cmd/main.go
./bin/manager --health-probe-bind-address=:18081 --metrics-bind-address=0 &

E2E_ENABLED=true E2E_KUBECONTEXT=kind-ak-bump \
  AUTHENTIK_URL=http://localhost:9000 \
  AUTHENTIK_TOKEN=authentik-operator-e2e-bootstrap-token \
  go test ./test/e2e/... -count=1 -timeout 15m
```

**Run the suite twice.** Fixed-name collisions and stale-status races only show
up on the second run.

Tear down afterwards, and restore the kubecontext you found:

```sh
./bin/kind delete cluster --name ak-bump
just authentik-down
kubectl config use-context <the original context>
```

### 7. Release

Per the version policy, a new authentik **series** means a new minor release of
the operator and chart. A newer patch within the series already pinned is a
patch release.

Release each series before starting the next one. Walking two series and
releasing once leaves the intermediate series with no operator anyone can
install, and the tables in step 4 with a row pointing at a release that does not
support it. Bump `version` and `appVersion` in
`charts/authentik-operator/Chart.yaml`, then tag — the release workflow builds
the multi-arch image, pushes the chart as an OCI artifact and attaches the
manifests.

## Checklist

- [ ] New authentik image tag verified to exist
- [ ] `just check-api-drift --candidate <series>` read before starting
- [ ] Client bumped, `go mod tidy` clean
- [ ] Every compile error fixed by reading the generated models
- [ ] Enums re-extracted and markers updated
- [ ] `supported-versions.yaml`, the Go gate, and both generated tables agree
- [ ] `releases:` gained an entry with its `tag`, so the older-authentik lookup
      points somewhere real
- [ ] The running authentik reported the expected version before any test was
      believed
- [ ] `just verify` passes on a clean tree
- [ ] `just test` and `just lint` pass
- [ ] E2E green against the new authentik, run **twice**
- [ ] Test environment torn down, kubecontext restored
- [ ] Chart version bumped for the new release
- [ ] One release cut per authentik series crossed, oldest first

## Failure modes seen before

- **Guessed a version that does not exist.** Always query the tag list.
- **Bumped the matrix without bumping the client**, so CI failed with
  `no value given for required property ...`. They move together.
- **Widened `supported-versions.yaml` to claim a range** the client cannot serve.
  One series per release.
- **Trusted a green E2E run that skipped every scenario.** Check the job summary
  reports executed scenarios, not just a passing exit code. Counting them works:
  `go test ./test/e2e/... -v | grep -c '^--- PASS'`.
- **Tested the old version while believing it was the new one.** The image tag
  lived in five places and only three were checked. The suite passed, against
  the version it was supposed to be replacing. Read
  `/api/v3/admin/version/` after `just authentik-up`.
- **Assumed a bump was available because time had passed.** authentik ships
  roughly quarterly; `2026.8` was still the newest series months after release.
  Query the tag list before planning a retarget.

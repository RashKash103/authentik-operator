## What does this change?

<!-- One or two sentences. What behaviour is different after this PR? -->

## Why?

<!-- Link the issue if there is one: Fixes #123 -->

## Type of change

- [ ] Bug fix (non-breaking)
- [ ] New feature (non-breaking)
- [ ] Breaking change (API/CRD field removed or changed meaning, RBAC widened, behaviour users depend on changed)
- [ ] Docs / chart / CI only

## API and compatibility

<!-- Delete this section if you did not touch api/ or supported-versions.yaml. -->

- [ ] CRD types changed. `make manifests generate` was run and the regenerated files are committed.
- [ ] `supported-versions.yaml` changed. `make sync-versions` was run and the derived files are committed.
- [ ] This is safe to apply on top of the previous release without manual steps. If not, upgrade notes are included below.

## Testing

<!--
Which authentik versions did you actually run against? The E2E matrix covers
every series in supported-versions.yaml, but say what you verified locally.
-->

- [ ] `make test` passes
- [ ] `make lint` passes
- [ ] `make verify` passes (generated artifacts are current)
- [ ] `make test-e2e` passes against a local authentik (`make authentik-up`)

authentik version(s) tested against:

## Checklist

- [ ] New behaviour is covered by tests
- [ ] User-facing changes are documented (README / `docs/`)
- [ ] Chart values changed? `values.yaml`, `values.schema.json` and the generated chart README are all updated
- [ ] No secrets, tokens or customer data in the diff, logs or fixtures

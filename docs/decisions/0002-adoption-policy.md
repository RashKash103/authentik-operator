# 0002 — `spec.adoptionPolicy` defaults to `FailOnConflict`

- **Status:** Accepted
- **Date:** <!-- PLACEHOLDER: date the owner accepted this decision -->
- **Deciders:** Project owner
- **Supersedes:** —
- **Superseded by:** —

## Context

authentik identifies most objects by a human-chosen name or slug, not by
anything the operator controls. An application has a `slug`; a provider has a
`name`. These are the natural identifiers a user will pick, and they are the
identifiers the authentik API searches by.

Kubernetes, by contrast, identifies objects by UID and lets the operator stamp
ownership onto anything it creates. The operator has no equivalent lever on the
authentik side: there is no field that means "this object belongs to
authentik-operator, in this cluster, on behalf of this custom resource" unless
the operator invents one and writes it there itself.

So the central reconcile question — *does the thing I am about to create already
exist?* — resolves to a name lookup, and a name lookup cannot distinguish
between these cases:

1. The operator created this object on a previous reconcile and is seeing its
   own work. (Normal. Must be an update.)
2. The operator created this object, then its own custom resource was deleted
   and recreated, so the link was lost. (Recoverable. Wants adoption.)
3. A human created this object by hand, months ago, and has been tuning it ever
   since. (Emphatically not ours.)
4. A different cluster, or a different operator installation, pointed at the
   same authentik, created it. (Not ours, and two controllers fighting over one
   object is the worst outcome available.)
5. A completely unrelated object happens to share the name. `grafana`,
   `gitlab`, `proxy` are not distinctive.

Name collisions are therefore not an edge case; they are routine. Two clusters
sharing one authentik will collide. A staging and production namespace both
creating an application slugged `grafana` will collide. A user who deletes a
custom resource with a retain policy and reapplies it will collide.

The question is what the operator does in that situation **by default**, before
the user has thought about any of this.

Silent adoption — find the existing object, assume it is ours, overwrite its
fields to match the spec — is a tempting default because it makes the happy path
smooth and makes case 2 self-healing. It is also how an operator quietly takes
over a hand-tuned production object and destroys work that nobody asked it to
touch. The damage is worse than a normal misconfiguration: the operator does not
merely fail to do what was wanted, it actively overwrites something that was
already correct, and it does so with no signal beyond a successful reconcile.
For an identity provider, a silently rewritten OAuth2 provider means rotated
client secrets and a broken production login; a silently rewritten SAML provider
means forged-looking assertions and a broken federation.

The asymmetry is decisive. Refusing to act when adoption was wanted costs one
confused user, one error condition, and one field to set. Acting when adoption
was not wanted costs a production outage and destroyed configuration that may
not be recoverable.

## Options considered

### Option A — Always adopt

On a name match, take ownership and reconcile the object to spec.

- **For:** Smoothest experience. Case 2 self-heals. Importing existing authentik
  configuration into the operator is as easy as writing a matching manifest.
- **Against:** Cases 3, 4 and 5 are silent data loss. Two operator installations
  pointed at one authentik will fight, each reverting the other's writes on
  every reconcile, forever. The blast radius lands on the identity provider,
  which is the single worst place in an infrastructure to have a surprise.

### Option B — Never adopt; always fail on a pre-existing object

On a name match that the operator cannot prove it created, set a failure
condition and stop.

- **For:** The operator can never damage something it did not create. Trivially
  auditable.
- **Against:** No migration path at all. Anyone with an existing authentik has
  to delete objects by hand before the operator will manage them, which for a
  live provider means a deliberate outage. Case 2 — the operator's own prior
  work, orphaned by a custom-resource recreate — becomes permanently stuck with
  no in-band remedy, which is a bad failure mode for a normal `kubectl delete`
  and reapply.

### Option C — Fail by default, adopt on explicit opt-in (chosen)

A `spec.adoptionPolicy` field with values `FailOnConflict` (the default) and
`AdoptExisting`.

- **For:** Safe by default, with an in-band escape hatch. The user who genuinely
  wants to import an existing object says so, in the manifest, in a way that
  shows up in a diff and in a code review. The failure condition can name the
  exact field to set, so discovery costs one read of the error message.
- **Against:** An extra field on every managed kind. Users importing a large
  existing authentik must set it repeatedly. And it is a one-way door in
  practice: once adopted, the object is managed, and the policy field no longer
  does anything.

## Decision

**Every managed kind carries `spec.adoptionPolicy`, and it defaults to
`FailOnConflict`.**

```yaml
spec:
  adoptionPolicy: FailOnConflict   # default
  # adoptionPolicy: AdoptExisting  # explicit opt-in
```

Semantics:

- **`FailOnConflict` (default).** If an authentik object with the target
  name/slug exists and the operator cannot establish that it created it, the
  reconcile fails. The operator sets a `Ready=False` condition with reason
  `AdoptionRequired`, emits a warning event, **modifies nothing in authentik**,
  and requeues with backoff. If the conflicting object is later removed, the
  next reconcile proceeds normally without user intervention.

- **`AdoptExisting`.** If such an object exists, the operator takes ownership,
  stamps its provenance marker (see below), and reconciles it to the spec on
  this and every subsequent pass. Fields the operator manages are overwritten;
  fields outside the CRD's schema are left alone where the authentik API permits
  a partial update.

Supporting rules:

1. **Provenance is recorded in authentik, not inferred.** On create or adopt,
   the operator writes a marker identifying the managing installation and the
   source custom resource — an attribute the authentik object type supports, or
   a structured marker in a description-style field where it does not. "I
   created this" is then a positive check rather than a guess from a matching
   name.

2. **Case 1 is not a conflict.** An object carrying this installation's own
   provenance marker for this custom resource is the operator's own work; it is
   updated normally, regardless of `adoptionPolicy`.

3. **A foreign provenance marker is a conflict even under `AdoptExisting`** if
   it names a *different* managing installation. Two operators overwriting each
   other in a loop is worse than a clear error, so this case fails loudly and
   names the other installation in the condition message.

4. **The policy governs acquisition, not release.** Once managed, an object
   stays managed until the custom resource is deleted; changing
   `adoptionPolicy` to `FailOnConflict` afterwards does not orphan it. Deletion
   behaviour is a separate concern and a separate field.

5. **The error message must be actionable.** The `AdoptionRequired` condition
   names the conflicting authentik object, its type, and the exact field to set
   to proceed. A safety default that users cannot get past is a bug in the
   safety default.

## Consequences

### Positive

- The operator cannot silently overwrite a hand-tuned authentik object. Taking
  over an existing object is always an explicit, greppable, reviewable statement
  in a manifest.
- Two clusters pointed at one authentik discover the collision as an error
  rather than as a reconcile war.
- The opt-in is per-object, so importing one legacy provider does not lower the
  bar for everything else in the same namespace.
- `kubectl describe` on a stuck resource explains both the problem and the fix,
  which keeps the safe default from becoming a support burden.

### Negative

- Importing an existing authentik configuration means setting
  `adoptionPolicy: AdoptExisting` on every resource. For a large import that is
  tedious, and tedium invites a blanket `AdoptExisting` applied everywhere by
  templating — which quietly recreates Option A's risk profile. Documentation
  should warn against templating this field globally.
- Users who delete and recreate a custom resource hit `AdoptionRequired` on
  objects the operator itself created moments earlier, unless the provenance
  marker survived — which it should, but this depends on rule 1 being
  implemented for every authentik object type, including those whose API offers
  no obviously suitable field.
- Rule 1 costs an extra write on create and an extra read on reconcile, and it
  puts operator-internal metadata in a field a human may edit in the authentik
  UI. A user clearing that field converts case 1 into case 3 and gets an
  unexpected `AdoptionRequired`.
- `FailOnConflict` requeues with backoff rather than giving up, so a
  long-standing conflict is a permanently non-Ready resource generating events.
  Alerting on operator resource readiness will need to account for the
  deliberately-stuck case.

### Follow-up work

- Decide the provenance marker's exact shape per authentik object type, and
  record it in a follow-up ADR if it turns out not to be uniform.
- E2E coverage for: a pre-existing object under `FailOnConflict` (must not be
  modified — assert the object's fields are byte-identical after the failed
  reconcile), the same under `AdoptExisting`, and a foreign-marker conflict
  under `AdoptExisting`.
- A `kubectl` plugin or documented procedure for bulk import, so that users with
  a large existing authentik are not driven toward a global `AdoptExisting`.

## References

- [`SECURITY.md`](../../SECURITY.md) — why overwriting an identity-provider
  object is a higher-consequence failure than the usual reconcile mistake.
- [ADR 0001](0001-connection-scope.md) — the other safety default in the API.

# chart-testing values

**Every `.yaml` file in this directory is installed by `ct install` against a
real cluster.** They must therefore be installable without any setup beyond the
namespace chart-testing creates itself.

That rules out, among others:

- a `serviceAccount.create: false` naming an account nobody has created;
- `secret` volumes that are not `optional`, since the Secret cannot be
  pre-created in a namespace that does not exist yet;
- images from registries the cluster cannot pull from, and digests that do not
  resolve;
- replica counts with required anti-affinity that cannot schedule on a
  single-node kind cluster.

Configurations of that shape are still worth rendering. Put them in
`../test-values/`, which `helm template` covers in the lint job and which
chart-testing never touches.

Two failures have already come from ignoring this, and neither was visible to
`helm lint` or `helm template` — only a real install finds them.

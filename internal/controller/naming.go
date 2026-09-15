/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

// ScopedName returns the name an object carries in authentik.
//
// Several operators can share one authentik - one per cluster, or one reaching
// it directly while another goes through a proxy. authentik has no ownership
// marker on providers or applications, so the only way for an operator to tell
// its own objects from another's is to scope the name it uses.
//
// With a cluster identity set, a provider named "grafana" is created as
// "grafana-prod-eu". Lookups use the same scoped name, so an operator never
// finds, adopts or deletes an object belonging to a different cluster, and two
// clusters get two objects rather than silently fighting over one.
//
// The suffix is slug-safe, because application slugs accept only
// [-a-zA-Z0-9_] and the same rule has to work for them.
func ScopedName(cluster, name string) string {
	if cluster == "" {
		return name
	}
	return name + "-" + cluster
}

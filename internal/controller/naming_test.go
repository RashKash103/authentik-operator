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

import (
	"regexp"
	"testing"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
)

// Application slugs accept only [-a-zA-Z0-9_], and the scoped name has to work
// for them as well as for provider names.
var slugSafe = regexp.MustCompile(`^[-a-zA-Z0-9_]+$`)

func TestScopedName(t *testing.T) {
	cases := []struct {
		name    string
		cluster string
		object  string
		want    string
	}{
		{
			// The single-operator case, which must stay exactly as it was:
			// adding a cluster identity later is opt-in, and an unset one must
			// not rename anything already created.
			name:    "no cluster leaves the name untouched",
			cluster: "",
			object:  "grafana",
			want:    "grafana",
		},
		{
			name:    "a cluster scopes the name",
			cluster: "prod-eu",
			object:  "grafana",
			want:    "grafana-prod-eu",
		},
		{
			// Two operators sharing one authentik is the whole point: the same
			// declared name must produce two distinct authentik objects.
			name:    "a different cluster produces a different object",
			cluster: "prod-us",
			object:  "grafana",
			want:    "grafana-prod-us",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ScopedName(tc.cluster, tc.object)
			if got != tc.want {
				t.Errorf("ScopedName(%q, %q) = %q, want %q", tc.cluster, tc.object, got, tc.want)
			}
			if !slugSafe.MatchString(got) {
				t.Errorf("%q is not usable as an application slug", got)
			}
		})
	}
}

// Distinctness is the property that matters; assert it directly rather than
// inferring it from the table above.
func TestScopedNamesAreDistinctPerCluster(t *testing.T) {
	a := ScopedName("prod-eu", "grafana")
	b := ScopedName("prod-us", "grafana")
	unscoped := ScopedName("", "grafana")

	if a == b {
		t.Error("two clusters produced the same authentik object name")
	}
	if a == unscoped || b == unscoped {
		t.Error("a scoped name collided with the unscoped one")
	}
}

// TestApplicationSlugIsNotScoped guards a deliberate exclusion.
//
// An application's slug appears in the URL users are sent to when logging in,
// and its name in every user's application list. Scoping either would publish
// the operator's cluster topology to anyone who reaches that page, signed in or
// not. Provider names are admin-only and are scoped; applications are not.
func TestApplicationSlugIsNotScoped(t *testing.T) {
	app := &authentikv1alpha1.Application{}
	app.Spec.Slug = "grafana"

	adapter := &applicationAdapter{
		client: &stubAuthentikClient{cluster: "prod-eu"},
		app:    app,
	}

	if got := adapter.DesiredName(); got != "grafana" {
		t.Errorf("DesiredName() = %q, want the unscoped slug: a cluster identity "+
			"must not reach a user-visible URL", got)
	}
}

// The converse: provider names are admin-only, so they are scoped.
func TestProviderNameIsScoped(t *testing.T) {
	provider := &authentikv1alpha1.OAuth2Provider{}
	provider.Name = "grafana"

	adapter := &oauth2Adapter{
		client:   &stubAuthentikClient{cluster: "prod-eu"},
		provider: provider,
	}

	if got := adapter.DesiredName(); got != "grafana-prod-eu" {
		t.Errorf("DesiredName() = %q, want the scoped provider name", got)
	}
}

// status.remoteName is documented as the name last observed in authentik, and
// callers use it to find the object in the UI. Reporting the declared name
// instead sends them looking for something that is not there, and the mismatch
// is invisible until a cluster identity is actually set.
func TestStatusReportsTheScopedName(t *testing.T) {
	provider := &authentikv1alpha1.OAuth2Provider{}
	provider.Name = "grafana"

	adapter := &oauth2Adapter{
		client:   &stubAuthentikClient{cluster: "prod-eu"},
		provider: provider,
	}

	r := &OAuth2ProviderReconciler{}
	r.recordIdentity(provider, SyncOutcome{RemoteID: "12"}, adapter)

	if got := provider.Status.RemoteName; got != "grafana-prod-eu" {
		t.Errorf("status.remoteName = %q, want the name authentik actually holds", got)
	}
}

// Applications are not scoped, so their status must report the bare slug --
// otherwise it would disagree with the URL users are sent to.
func TestApplicationStatusReportsTheBareSlug(t *testing.T) {
	app := &authentikv1alpha1.Application{}
	app.Name = "grafana"
	app.Spec.Slug = "grafana"

	adapter := &applicationAdapter{
		client: &stubAuthentikClient{cluster: "prod-eu"},
		app:    app,
	}

	r := &ApplicationReconciler{}
	r.recordIdentity(app, SyncOutcome{RemoteID: "grafana"}, adapter)

	if got := app.Status.RemoteName; got != "grafana" {
		t.Errorf("status.remoteName = %q, want the unscoped slug", got)
	}
}

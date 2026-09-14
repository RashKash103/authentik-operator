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
	"context"
	"slices"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

func newFakeScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := authentikv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("building scheme: %v", err)
	}
	return scheme
}

// ptr32 is a local helper so the tests can express an int32 status value
// inline without a package-level dependency.
func ptr32(v int32) *int32 { return &v }

// newTestApplication builds a minimal Application in namespace "apps".
func newTestApplication(refs ...authentikv1alpha1.ProviderReference) *authentikv1alpha1.Application {
	app := &authentikv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "grafana", Namespace: "apps"},
		Spec: authentikv1alpha1.ApplicationSpec{
			ConnectionRef: authentikv1alpha1.ConnectionReference{Name: "conn"},
			Slug:          "grafana",
		},
	}
	if len(refs) > 0 {
		app.Spec.ProviderRef = &refs[0]
		app.Spec.BackchannelProviderRefs = refs[1:]
	}
	return app
}

// oauth2Provider builds an OAuth2Provider, optionally already carrying the
// primary key authentik assigned it.
func oauth2Provider(namespace, name string, providerID *int32) *authentikv1alpha1.OAuth2Provider {
	return &authentikv1alpha1.OAuth2Provider{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Status: authentikv1alpha1.OAuth2ProviderStatus{
			ProviderStatus: authentikv1alpha1.ProviderStatus{ProviderID: providerID},
		},
	}
}

func newReconciler(t *testing.T, objects ...client.Object) *ApplicationReconciler {
	t.Helper()
	scheme := newFakeScheme(t)
	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&authentikv1alpha1.Application{},
			applicationProviderIndexKey, indexApplicationProviderRefs).
		WithObjects(objects...)
	return &ApplicationReconciler{Client: builder.Build(), Scheme: scheme}
}

// Ordering is the whole point of this resource: an Application is routinely
// applied in the same `kubectl apply -f .` as the provider it references, and
// may well be admitted first. Every one of these cases must requeue so the
// Application converges, never fail permanently.
func TestResolveProviderRefsRequeuesWhenReferenceIsNotResolvableYet(t *testing.T) {
	tests := []struct {
		name string
		// objects seeded into the cluster alongside the Application.
		objects []client.Object
		app     *authentikv1alpha1.Application
		// wantMessage fragments must all appear in the error, so that an
		// operator reading the condition learns exactly which reference is
		// holding the Application back.
		wantMessage []string
	}{
		{
			// The ordinary case: Application admitted before its provider.
			name:    "primary provider resource does not exist",
			objects: nil,
			app: newTestApplication(authentikv1alpha1.ProviderReference{
				Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "grafana-oauth",
			}),
			wantMessage: []string{"spec.providerRef", "OAuth2Provider", "grafana-oauth", "does not exist"},
		},
		{
			// The provider exists but its own controller has not reached
			// authentik yet, so there is no primary key to attach. Waiting is
			// correct; creating the Application without a provider is not.
			name:    "primary provider has no providerID yet",
			objects: []client.Object{oauth2Provider("apps", "grafana-oauth", nil)},
			app: newTestApplication(authentikv1alpha1.ProviderReference{
				Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "grafana-oauth",
			}),
			wantMessage: []string{"spec.providerRef", "grafana-oauth", "has not been created in authentik yet"},
		},
		{
			// A back-channel reference must name its own index, otherwise an
			// Application with several of them gives no clue which is broken.
			name: "back-channel reference names its index",
			objects: []client.Object{
				oauth2Provider("apps", "grafana-oauth", ptr32(7)),
				oauth2Provider("apps", "grafana-scim", ptr32(9)),
			},
			app: newTestApplication(
				authentikv1alpha1.ProviderReference{Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "grafana-oauth"},
				authentikv1alpha1.ProviderReference{Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "grafana-scim"},
				authentikv1alpha1.ProviderReference{Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "grafana-ldap"},
			),
			wantMessage: []string{"spec.backchannelProviderRefs[1]", "grafana-ldap", "does not exist"},
		},
		{
			// A provider in another namespace must read as absent rather than
			// resolve: otherwise anyone able to create an Application could
			// attach a provider owned by a different team.
			name:    "provider in another namespace is not visible",
			objects: []client.Object{oauth2Provider("other", "grafana-oauth", ptr32(7))},
			app: newTestApplication(authentikv1alpha1.ProviderReference{
				Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "grafana-oauth",
			}),
			wantMessage: []string{"spec.providerRef", "grafana-oauth", "does not exist"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newReconciler(t, tc.objects...)

			_, _, err := r.resolveProviderRefs(context.Background(), tc.app)
			if err == nil {
				t.Fatal("expected an unresolved reference error")
			}

			reason, requeue := ResultFor(err)
			if reason != authentikv1alpha1.ReasonReferenceNotFound {
				t.Errorf("reason = %q, want %q", reason, authentikv1alpha1.ReasonReferenceNotFound)
			}
			if !requeue {
				t.Error("an unresolved provider reference must requeue, not fail permanently")
			}
			for _, fragment := range tc.wantMessage {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("error %q does not name %q", err.Error(), fragment)
				}
			}
		})
	}
}

// Resolution goes through the referenced resource's status, never through a
// name lookup in authentik, so that renaming the provider on the authentik
// side cannot repoint the application at something else.
func TestResolveProviderRefsReadsTheReferencedStatus(t *testing.T) {
	r := newReconciler(t,
		oauth2Provider("apps", "grafana-oauth", ptr32(7)),
		oauth2Provider("apps", "grafana-scim", ptr32(9)),
		oauth2Provider("apps", "grafana-ldap", ptr32(11)),
	)
	app := newTestApplication(
		authentikv1alpha1.ProviderReference{Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "grafana-oauth"},
		authentikv1alpha1.ProviderReference{Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "grafana-scim"},
		authentikv1alpha1.ProviderReference{Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "grafana-ldap"},
	)

	primary, backchannel, err := r.resolveProviderRefs(context.Background(), app)
	if err != nil {
		t.Fatalf("resolveProviderRefs: %v", err)
	}
	if primary == nil || *primary != 7 {
		t.Errorf("primary = %v, want 7", primary)
	}
	// Order matters: authentik stores the list as given, and a reordered list
	// would look like drift on every reconcile.
	if want := []int32{9, 11}; !slices.Equal(backchannel, want) {
		t.Errorf("backchannel = %v, want %v", backchannel, want)
	}
}

// An Application with no providerRef is legitimate: a library-only entry that
// links somewhere without authentik protecting it.
func TestResolveProviderRefsAllowsNoProvider(t *testing.T) {
	r := newReconciler(t)

	primary, backchannel, err := r.resolveProviderRefs(context.Background(), newTestApplication())
	if err != nil {
		t.Fatalf("resolveProviderRefs: %v", err)
	}
	if primary != nil || backchannel != nil {
		t.Errorf("got (%v, %v), want (nil, nil)", primary, backchannel)
	}
}

// SAMLProvider and ProxyProvider are in the schema so the API does not have to
// break when they land, but they cannot be resolved yet. That is a spec the
// operator will never satisfy on its own, so it must not requeue forever: the
// user has to change the spec or wait for a release.
func TestResolveProviderRefsRejectsUnimplementedKindsWithoutSpinning(t *testing.T) {
	for _, kind := range []authentikv1alpha1.ProviderKind{
		authentikv1alpha1.ProviderKindSAML,
		authentikv1alpha1.ProviderKindProxy,
	} {
		t.Run(string(kind), func(t *testing.T) {
			r := newReconciler(t)
			app := newTestApplication(authentikv1alpha1.ProviderReference{Kind: kind, Name: "whatever"})

			_, _, err := r.resolveProviderRefs(context.Background(), app)
			if err == nil {
				t.Fatalf("expected %s to be rejected", kind)
			}

			reason, requeue := ResultFor(err)
			if reason != authentikv1alpha1.ReasonInvalidSpec {
				t.Errorf("reason = %q, want %q", reason, authentikv1alpha1.ReasonInvalidSpec)
			}
			if requeue {
				t.Error("an unimplemented provider kind will not fix itself, so it must not requeue")
			}
			if !authentik.IsValidation(err) {
				t.Errorf("error %v should classify as a validation failure", err)
			}
		})
	}
}

// A reference with no kind must behave as OAuth2Provider. The CRD default
// covers objects created through the API server, but an object deserialised
// from an older stored version, or built in code, can still arrive empty.
func TestResolveProviderRefsDefaultsToOAuth2Kind(t *testing.T) {
	r := newReconciler(t, oauth2Provider("apps", "grafana-oauth", ptr32(7)))
	app := newTestApplication(authentikv1alpha1.ProviderReference{Name: "grafana-oauth"})

	primary, _, err := r.resolveProviderRefs(context.Background(), app)
	if err != nil {
		t.Fatalf("resolveProviderRefs: %v", err)
	}
	if primary == nil || *primary != 7 {
		t.Errorf("primary = %v, want 7", primary)
	}
}

func TestIndexApplicationProviderRefs(t *testing.T) {
	tests := []struct {
		name string
		obj  client.Object
		want []string
	}{
		{
			name: "no references indexes nothing",
			obj:  newTestApplication(),
			want: []string{},
		},
		{
			// Both the primary and the back-channel references are indexed:
			// either provider becoming ready is a reason to reconcile.
			name: "primary and back-channel references are both indexed",
			obj: newTestApplication(
				authentikv1alpha1.ProviderReference{Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "a"},
				authentikv1alpha1.ProviderReference{Kind: authentikv1alpha1.ProviderKindSAML, Name: "b"},
			),
			want: []string{"OAuth2Provider/a", "SAMLProvider/b"},
		},
		{
			// The index key must agree with the default the map function
			// applies, or a reference written without a kind would never be
			// woken by its provider.
			name: "an empty kind indexes under OAuth2Provider",
			obj:  newTestApplication(authentikv1alpha1.ProviderReference{Name: "a"}),
			want: []string{"OAuth2Provider/a"},
		},
		{
			// The indexer is handed every object type the cache holds, so it
			// must tolerate one it does not understand.
			name: "a foreign object indexes nothing",
			obj:  oauth2Provider("apps", "a", nil),
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := indexApplicationProviderRefs(tc.obj)
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// The watch is what turns "applied out of order" from a wait into a non-event:
// without it an Application whose provider was not ready sits on a 30s backoff
// even though the provider became usable immediately.
func TestApplicationsForProviderEnqueuesOnlyTheWaitingApplications(t *testing.T) {
	waiting := newTestApplication(authentikv1alpha1.ProviderReference{
		Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "grafana-oauth",
	})

	viaBackchannel := newTestApplication(
		authentikv1alpha1.ProviderReference{Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "other-oauth"},
		authentikv1alpha1.ProviderReference{Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "grafana-oauth"},
	)
	viaBackchannel.Name = "wiki"

	unrelated := newTestApplication(authentikv1alpha1.ProviderReference{
		Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "other-oauth",
	})
	unrelated.Name = "unrelated"

	// Same provider name, different namespace: it must not be woken, matching
	// the namespace scoping that resolution itself enforces.
	otherNamespace := newTestApplication(authentikv1alpha1.ProviderReference{
		Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "grafana-oauth",
	})
	otherNamespace.Namespace = "other"

	r := newReconciler(t, waiting, viaBackchannel, unrelated, otherNamespace)
	mapFunc := r.applicationsForProvider(authentikv1alpha1.ProviderKindOAuth2)

	got := mapFunc(context.Background(), oauth2Provider("apps", "grafana-oauth", ptr32(7)))

	want := []reconcile.Request{
		{NamespacedName: client.ObjectKey{Namespace: "apps", Name: "grafana"}},
		{NamespacedName: client.ObjectKey{Namespace: "apps", Name: "wiki"}},
	}
	slices.SortFunc(got, func(a, b reconcile.Request) int {
		return strings.Compare(a.Name, b.Name)
	})
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A provider that is deleted, or that loses its primary key, is precisely when
// a dependent Application needs to re-report its reference as broken, so the
// map function must not filter those events away.
func TestApplicationsForProviderEnqueuesEvenWithoutAProviderID(t *testing.T) {
	waiting := newTestApplication(authentikv1alpha1.ProviderReference{
		Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "grafana-oauth",
	})
	r := newReconciler(t, waiting)

	got := r.applicationsForProvider(authentikv1alpha1.ProviderKindOAuth2)(
		context.Background(), oauth2Provider("apps", "grafana-oauth", nil))

	if len(got) != 1 {
		t.Fatalf("got %d requests, want 1", len(got))
	}
}

// The index key and the lookup key are built by different code paths; if they
// ever disagree the watch silently stops firing, which is the kind of bug that
// only shows up as "sometimes it takes 30 seconds".
func TestProviderReferenceKeyIsStable(t *testing.T) {
	ref := authentikv1alpha1.ProviderReference{Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "x"}
	defaulted := authentikv1alpha1.ProviderReference{Name: "x"}

	if ref.Key() != defaulted.Key() {
		t.Errorf("%q != %q: an explicit OAuth2Provider kind must key the same as the default",
			ref.Key(), defaulted.Key())
	}
	if ref.Key() != "OAuth2Provider/x" {
		t.Errorf("Key() = %q, want %q", ref.Key(), "OAuth2Provider/x")
	}
}

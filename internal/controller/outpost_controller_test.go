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
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "goauthentik.io/api/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// registeredProvider builds an OAuth2Provider that has finished registering
// with authentik and therefore carries a primary key.
func registeredProvider(namespace, name string, id int32) *authentikv1alpha1.OAuth2Provider {
	provider := &authentikv1alpha1.OAuth2Provider{}
	provider.Name = name
	provider.Namespace = namespace
	provider.Status.ProviderID = &id
	return provider
}

// pendingProvider builds an OAuth2Provider that exists but has not been
// registered with authentik yet, which is the normal state mid-rollout.
func pendingProvider(namespace, name string) *authentikv1alpha1.OAuth2Provider {
	provider := &authentikv1alpha1.OAuth2Provider{}
	provider.Name = name
	provider.Namespace = namespace
	return provider
}

func oauth2Ref(name string) authentikv1alpha1.ProviderReference {
	return authentikv1alpha1.ProviderReference{
		Kind: authentikv1alpha1.ProviderKindOAuth2,
		Name: name,
	}
}

// TestResolveProviderIDsIsAllOrNothing is the case this controller exists to
// get right.
//
// A partially resolved provider set is the normal state during a rollout, and
// registering an outpost with it would leave the applications whose providers
// had not resolved yet completely unproxied: not denied, not erroring, just
// unprotected, with nothing anywhere saying so. So anything short of the full
// set must abort and requeue, leaving the outpost exactly as it was.
func TestResolveProviderIDsIsAllOrNothing(t *testing.T) {
	scheme := serviceConnectionScheme(t)

	cases := []struct {
		name        string
		refs        []authentikv1alpha1.ProviderReference
		objects     []client.Object
		wantIDs     []int32
		wantErr     bool
		wantInError string
	}{
		{
			name:    "no references resolve to an empty set",
			wantIDs: []int32{},
		},
		{
			name:    "every reference resolves, in spec order",
			refs:    []authentikv1alpha1.ProviderReference{oauth2Ref("grafana"), oauth2Ref("wiki")},
			objects: []client.Object{registeredProvider("team-a", "wiki", 9), registeredProvider("team-a", "grafana", 7)},
			wantIDs: []int32{7, 9},
		},
		{
			// The reference names a resource nobody has created. Naming it in
			// the error is what turns "the outpost is not ready" into an
			// actionable message.
			name:        "a missing provider resource aborts the whole set",
			refs:        []authentikv1alpha1.ProviderReference{oauth2Ref("grafana"), oauth2Ref("absent")},
			objects:     []client.Object{registeredProvider("team-a", "grafana", 7)},
			wantErr:     true,
			wantInError: "absent",
		},
		{
			// The mid-rollout case: the provider resource is there, but its
			// own controller has not registered it with authentik yet, so
			// there is no primary key to attach.
			name:        "a provider still registering aborts the whole set",
			refs:        []authentikv1alpha1.ProviderReference{oauth2Ref("grafana"), oauth2Ref("wiki")},
			objects:     []client.Object{registeredProvider("team-a", "grafana", 7), pendingProvider("team-a", "wiki")},
			wantErr:     true,
			wantInError: "wiki",
		},
		{
			// A provider of the same name in another namespace must not
			// satisfy the reference; that would let an Outpost in one
			// namespace attach itself to another team's provider.
			name:        "a provider in another namespace does not resolve",
			refs:        []authentikv1alpha1.ProviderReference{oauth2Ref("grafana")},
			objects:     []client.Object{registeredProvider("team-b", "grafana", 7)},
			wantErr:     true,
			wantInError: "grafana",
		},
		{
			// Kinds the operator cannot resolve yet are reported as
			// unresolved, never quietly dropped: dropping one is exactly the
			// partial-set failure this function exists to prevent.
			name:        "an unsupported provider kind aborts the whole set",
			refs:        []authentikv1alpha1.ProviderReference{oauth2Ref("grafana"), {Kind: authentikv1alpha1.ProviderKindSAML, Name: "sso"}},
			objects:     []client.Object{registeredProvider("team-a", "grafana", 7)},
			wantErr:     true,
			wantInError: "SAMLProvider",
		},
		{
			// A reference written without a kind is an OAuth2Provider, which
			// is what the CRD default fills in. The resolver applies the same
			// default so a hand-built object behaves identically.
			name:    "an omitted kind defaults to OAuth2Provider",
			refs:    []authentikv1alpha1.ProviderReference{{Name: "grafana"}},
			objects: []client.Object{registeredProvider("team-a", "grafana", 7)},
			wantIDs: []int32{7},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &OutpostReconciler{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(tc.objects...).Build(),
				Scheme: scheme,
			}
			outpost := newOutpost(tc.refs...)

			ids, err := r.resolveProviderIDs(context.Background(), outpost)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got ids %v", ids)
				}
				if ids != nil {
					t.Errorf("ids = %v, want nothing: a partial set must never reach the adapter", ids)
				}
				if !strings.Contains(err.Error(), tc.wantInError) {
					t.Errorf("err = %q, want it to name %q", err, tc.wantInError)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveProviderIDs: %v", err)
			}
			if !slices.Equal(ids, tc.wantIDs) {
				t.Errorf("ids = %v, want %v", ids, tc.wantIDs)
			}
		})
	}
}

// An unresolved reference is "not yet", not "never": the provider will finish
// registering, so the outpost has to keep retrying rather than wedging until
// someone edits the spec.
func TestUnresolvedProviderRequeues(t *testing.T) {
	scheme := serviceConnectionScheme(t)
	r := &OutpostReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	_, err := r.resolveProviderIDs(context.Background(), newOutpost(oauth2Ref("grafana")))
	if err == nil {
		t.Fatal("expected an error for a missing provider")
	}
	if !authentik.IsNotFound(err) {
		t.Fatalf("err = %v, want a not-found", err)
	}

	reason, requeue := ResultFor(err)
	if reason != authentikv1alpha1.ReasonReferenceNotFound || !requeue {
		t.Errorf("ResultFor = (%s, %v), want (ReferenceNotFound, true)", reason, requeue)
	}
}

// An outpost has to be reconciled the moment a provider it waits on finishes
// registering. Without the index and the watch it would sit out the requeue
// timer instead, which is the difference between an application coming back in
// a second and coming back in half a minute.
func TestOutpostsAreWokenByTheProvidersTheyReference(t *testing.T) {
	scheme := serviceConnectionScheme(t)

	waiting := newOutpost(oauth2Ref("grafana"))
	waiting.Name = "edge"

	unrelated := newOutpost(oauth2Ref("wiki"))
	unrelated.Name = "other"

	// A same-named provider of a different kind must not wake the outpost,
	// which is why the index key carries the kind.
	sameNameOtherKind := newOutpost(authentikv1alpha1.ProviderReference{
		Kind: authentikv1alpha1.ProviderKindSAML, Name: "grafana",
	})
	sameNameOtherKind.Name = "saml-edge"

	r := &OutpostReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(waiting, unrelated, sameNameOtherKind).
			WithIndex(&authentikv1alpha1.Outpost{}, outpostProviderRefIndexKey, indexOutpostProviderRefs).
			Build(),
		Scheme: scheme,
	}

	provider := registeredProvider("team-a", "grafana", 7)
	requests := r.outpostsForProvider(authentikv1alpha1.ProviderKindOAuth2)(context.Background(), provider)

	want := []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: "team-a", Name: "edge"}}}
	if !slices.Equal(requests, want) {
		t.Errorf("requests = %v, want exactly %v", requests, want)
	}
}

// The index has to key on the same value the map function looks up, including
// the default kind, or a reference written without one is never woken.
func TestIndexOutpostProviderRefsKeysOnKindAndName(t *testing.T) {
	outpost := newOutpost(
		authentikv1alpha1.ProviderReference{Name: "grafana"},
		authentikv1alpha1.ProviderReference{Kind: authentikv1alpha1.ProviderKindProxy, Name: "wiki"},
	)

	got := indexOutpostProviderRefs(outpost)
	want := []string{"OAuth2Provider/grafana", "ProxyProvider/wiki"}
	if !slices.Equal(got, want) {
		t.Errorf("index keys = %v, want %v", got, want)
	}

	// Anything that is not an Outpost must index to nothing rather than panic;
	// the indexer is handed whatever the cache holds.
	if keys := indexOutpostProviderRefs(&corev1.Secret{}); keys != nil {
		t.Errorf("index keys for a non-Outpost = %v, want none", keys)
	}
}

// The token is what makes the Outpost CRD useful for a self-hosted outpost, so
// it has to reach a Secret the workload can mount. It must never reach status,
// which is readable by anyone who can read the custom resource.
func TestWriteTokenPublishesSecretAndKeepsItOutOfStatus(t *testing.T) {
	scheme := serviceConnectionScheme(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key":"ak-outpost-secret-value"}`))
	}))
	defer server.Close()

	akClient, err := authentik.New(authentik.Config{BaseURL: server.URL, Token: "operator-token"})
	if err != nil {
		t.Fatalf("building the authentik client: %v", err)
	}

	outpost := newOutpost()
	outpost.Spec.WriteTokenTo = &authentikv1alpha1.OutpostTokenSecretRef{Name: "edge-outpost"}

	r := &OutpostReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(outpost).Build(),
		Scheme: scheme,
	}
	adapter := &outpostAdapter{
		client:   akClient,
		outpost:  outpost,
		observed: &api.Outpost{Pk: "uuid", Name: "edge", TokenIdentifier: "ak-outpost-edge"},
	}

	if err := r.writeToken(context.Background(), outpost, adapter); err != nil {
		t.Fatalf("writeToken: %v", err)
	}

	var secret corev1.Secret
	key := types.NamespacedName{Namespace: "team-a", Name: "edge-outpost"}
	if err := r.Get(context.Background(), key, &secret); err != nil {
		t.Fatalf("reading the published secret: %v", err)
	}

	if got := string(secret.Data["token"]); got != "ak-outpost-secret-value" {
		t.Errorf("token key = %q, want the value read from authentik", got)
	}
	// An outpost needs somewhere to send the token as well as the token, so
	// the base URL travels with it.
	if got := string(secret.Data["authentik-host"]); got != server.URL {
		t.Errorf("authentik-host key = %q, want %q", got, server.URL)
	}
	// Owned by the outpost, so deleting the outpost takes the credential with
	// it rather than leaving a live token lying around.
	if len(secret.OwnerReferences) != 1 || secret.OwnerReferences[0].Name != "edge" {
		t.Errorf("owner references = %v, want the outpost", secret.OwnerReferences)
	}

	if outpost.Status.TokenSecretName != "edge-outpost" {
		t.Errorf("status.tokenSecretName = %q, want the Secret name", outpost.Status.TokenSecretName)
	}
	if strings.Contains(outpost.Status.TokenIdentifier, "secret-value") {
		t.Fatal("the token value must never be recorded in status")
	}
}

// Without writeTokenTo nothing is published and, just as importantly, no token
// is fetched: reading one needs a permission the operator may not have, and
// failing over an optional feature would take the whole resource down.
func TestWriteTokenIsSkippedWhenNotRequested(t *testing.T) {
	scheme := serviceConnectionScheme(t)
	outpost := newOutpost()

	r := &OutpostReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(outpost).Build(),
		Scheme: scheme,
	}
	// A nil client would panic if the token were fetched, which is the
	// assertion: nothing must be fetched at all.
	adapter := &outpostAdapter{outpost: outpost}

	if err := r.writeToken(context.Background(), outpost, adapter); err != nil {
		t.Fatalf("writeToken: %v", err)
	}
	if outpost.Status.TokenSecretName != "" {
		t.Error("nothing should have been published")
	}
}

func TestRecordIdentityWritesResolvedReferences(t *testing.T) {
	outpost := newOutpost(oauth2Ref("grafana"))
	r := &OutpostReconciler{}
	adapter := &outpostAdapter{
		outpost:  outpost,
		observed: &api.Outpost{Pk: "uuid", TokenIdentifier: "ak-outpost-edge"},
	}

	r.recordIdentity(outpost, SyncOutcome{RemoteID: "uuid"}, adapter, []int32{7}, "sc-uuid")

	if outpost.Status.OutpostID != "uuid" || outpost.Status.RemoteID != "uuid" {
		t.Errorf("status = %+v, want the UUID recorded", outpost.Status)
	}
	if !slices.Equal(outpost.Status.ProviderIDs, []int32{7}) {
		t.Errorf("status.providerIDs = %v, want the resolved set", outpost.Status.ProviderIDs)
	}
	if outpost.Status.ServiceConnectionID != "sc-uuid" {
		t.Errorf("status.serviceConnectionID = %q, want the resolved UUID", outpost.Status.ServiceConnectionID)
	}
	// The identifier names the token; it is not the token, and it is what an
	// administrator needs to find it in authentik.
	if outpost.Status.TokenIdentifier != "ak-outpost-edge" {
		t.Errorf("status.tokenIdentifier = %q, want the observed identifier", outpost.Status.TokenIdentifier)
	}
	if outpost.Status.LastSyncedTime == nil {
		t.Error("lastSyncedTime must be stamped on a successful sync")
	}
}

// An outpost with no serviceConnectionRef is registered without one, which is
// what a self-hosted outpost wants; it must not turn into a lookup that fails.
func TestResolveServiceConnectionSkipsEmptyReference(t *testing.T) {
	r := &OutpostReconciler{}

	got, err := r.resolveServiceConnection(context.Background(), newOutpost())
	if err != nil || got != "" {
		t.Fatalf("resolveServiceConnection = (%q, %v), want (\"\", nil)", got, err)
	}
}

// Service connections resolve through the referenced resource's status rather
// than by name lookup in authentik, so the Kubernetes object stays the source
// of truth and renaming the connection inside authentik cannot silently
// repoint an outpost.
func TestResolveServiceConnectionResolvesThroughResource(t *testing.T) {
	scheme := serviceConnectionScheme(t)
	outpost := newOutpost()
	outpost.Spec.ServiceConnectionRef = &authentikv1alpha1.ServiceConnectionReference{
		KubernetesServiceConnectionName: "prod-cluster",
	}

	ready := &authentikv1alpha1.KubernetesServiceConnection{
		ObjectMeta: metav1.ObjectMeta{Name: "prod-cluster", Namespace: outpost.Namespace},
	}
	ready.Status.ServiceConnectionID = "sc-uuid"

	r := &OutpostReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(ready).Build(),
	}

	got, err := r.resolveServiceConnection(context.Background(), outpost)
	if err != nil {
		t.Fatalf("resolveServiceConnection: %v", err)
	}
	if got != "sc-uuid" {
		t.Errorf("got %q, want the resolved UUID", got)
	}
}

// An outpost applied before its service connection must requeue rather than
// fail, the same as every other reference in this API.
func TestResolveServiceConnectionRequeuesWhenNotReady(t *testing.T) {
	scheme := serviceConnectionScheme(t)
	outpost := newOutpost()
	outpost.Spec.ServiceConnectionRef = &authentikv1alpha1.ServiceConnectionReference{
		KubernetesServiceConnectionName: "not-there-yet",
	}

	r := &OutpostReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	_, err := r.resolveServiceConnection(context.Background(), outpost)
	if err == nil {
		t.Fatal("expected an unresolved reference to fail")
	}
	reason, requeue := ResultFor(err)
	if reason != authentikv1alpha1.ReasonReferenceNotFound || !requeue {
		t.Errorf("ResultFor = (%s, %v), want (ReferenceNotFound, true)", reason, requeue)
	}
}

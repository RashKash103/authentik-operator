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

// memberProvider builds an OAuth2Provider that names an outpost. id is nil for
// a provider that exists but has not registered with authentik yet, which is
// the normal state mid-rollout.
func memberProvider(namespace, name string, id *int32, outpost string) *authentikv1alpha1.OAuth2Provider {
	provider := &authentikv1alpha1.OAuth2Provider{}
	provider.Name = name
	provider.Namespace = namespace
	provider.Spec.OutpostRefs = []authentikv1alpha1.OutpostReference{{Name: outpost}}
	provider.Status.ProviderID = id
	provider.Status.AuthentikURL = "https://authentik.example"
	return provider
}

// TestDesiredProviderIDsIsAllOrNothing is the case this controller exists to
// get right.
//
// A partially resolved provider set is the normal state during a rollout, and
// registering an outpost with it would leave the applications whose providers
// had not resolved yet completely unproxied: not denied, not erroring, just
// unprotected, with nothing anywhere saying so. So anything short of the full
// set must abort and requeue, leaving the outpost exactly as it was.
func TestDesiredProviderIDsIsAllOrNothing(t *testing.T) {
	scheme := serviceConnectionScheme(t)

	cases := []struct {
		name        string
		objects     []client.Object
		wantIDs     []int32
		wantErr     bool
		wantInError string
	}{
		{
			name:    "no provider names this outpost",
			wantIDs: []int32{},
		},
		{
			// Sorted by provider name, not by listing order, so the set sent to
			// authentik does not churn and look like drift on every reconcile.
			name: "every provider that names the outpost is collected",
			objects: []client.Object{
				memberProvider("team-a", "wiki", ptr32(9), "edge"),
				memberProvider("team-a", "grafana", ptr32(7), "edge"),
			},
			wantIDs: []int32{7, 9},
		},
		{
			// The mid-rollout case: the provider resource names the outpost but
			// its own controller has not registered it yet, so there is no
			// primary key to attach.
			name: "a provider still registering aborts the whole set",
			objects: []client.Object{
				memberProvider("team-a", "grafana", ptr32(7), "edge"),
				memberProvider("team-a", "wiki", nil, "edge"),
			},
			wantErr:     true,
			wantInError: "wiki",
		},
		{
			// A provider naming a different outpost must not be collected.
			name: "providers naming another outpost are ignored",
			objects: []client.Object{
				memberProvider("team-a", "grafana", ptr32(7), "edge"),
				memberProvider("team-a", "wiki", ptr32(9), "other"),
			},
			wantIDs: []int32{7},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &OutpostReconciler{
				Client: fake.NewClientBuilder().WithScheme(scheme).
					WithObjects(tc.objects...).
					WithIndex(&authentikv1alpha1.OAuth2Provider{}, providerOutpostRefIndexKey, indexProviderOutpostRefs).
					WithIndex(&authentikv1alpha1.SAMLProvider{}, providerOutpostRefIndexKey, indexProviderOutpostRefs).
					WithIndex(&authentikv1alpha1.ProxyProvider{}, providerOutpostRefIndexKey, indexProviderOutpostRefs).
					Build(),
				Scheme: scheme,
			}

			ids, err := r.desiredProviderIDs(context.Background(), newOutpost(), testAuthentikClient())

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
				t.Fatalf("desiredProviderIDs: %v", err)
			}
			if !slices.Equal(ids, tc.wantIDs) {
				t.Errorf("ids = %v, want %v", ids, tc.wantIDs)
			}
		})
	}
}

// An unresolved member is "not yet", not "never": the provider will finish
// registering, so the outpost has to keep retrying rather than wedging until
// someone edits a spec.
func TestUnresolvedMemberRequeues(t *testing.T) {
	scheme := serviceConnectionScheme(t)
	r := &OutpostReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(memberProvider("team-a", "grafana", nil, "edge")).
			WithIndex(&authentikv1alpha1.OAuth2Provider{}, providerOutpostRefIndexKey, indexProviderOutpostRefs).
			WithIndex(&authentikv1alpha1.SAMLProvider{}, providerOutpostRefIndexKey, indexProviderOutpostRefs).
			WithIndex(&authentikv1alpha1.ProxyProvider{}, providerOutpostRefIndexKey, indexProviderOutpostRefs).
			Build(),
		Scheme: scheme,
	}

	_, err := r.desiredProviderIDs(context.Background(), newOutpost(), testAuthentikClient())
	if err == nil {
		t.Fatal("expected an error for a provider that has not registered")
	}
	if !authentik.IsNotFound(err) {
		t.Fatalf("err = %v, want a not-found", err)
	}

	reason, requeue := ResultFor(err)
	if reason != authentikv1alpha1.ReasonReferenceNotFound || !requeue {
		t.Errorf("ResultFor = (%s, %v), want (ReferenceNotFound, true)", reason, requeue)
	}
}

// mergeMembership is the whole point of inverting the relationship: an outpost
// can serve providers this operator knows nothing about, and a full-overwrite
// update would silently detach them. For a proxy outpost that means the
// application behind it stops being protected.
func TestMergeMembership(t *testing.T) {
	cases := []struct {
		name                      string
		current, managed, desired []int32
		want                      []int32
	}{
		{
			name:    "first reconcile attaches what is desired",
			desired: []int32{2, 1}, want: []int32{1, 2},
		},
		{
			// The case the record exists for. 9 was attached by hand; it is not
			// in managed, so it survives.
			name:    "a provider attached outside the operator is kept",
			current: []int32{9}, desired: []int32{1}, want: []int32{1, 9},
		},
		{
			// 1 was ours and its reference is gone, so it detaches. 9 was never
			// ours and stays.
			name:    "removing a reference detaches only what the operator attached",
			current: []int32{1, 9}, managed: []int32{1}, desired: nil, want: []int32{9},
		},
		{
			name:    "an unchanged set stays unchanged",
			current: []int32{1, 9}, managed: []int32{1}, desired: []int32{1}, want: []int32{1, 9},
		},
		{
			// A provider attached by hand and then also declared is not
			// duplicated; authentik would reject the repeat.
			name:    "a provider both attached by hand and declared appears once",
			current: []int32{1}, desired: []int32{1}, want: []int32{1},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeMembership(tc.current, tc.managed, tc.desired)
			if !slices.Equal(got, tc.want) {
				t.Errorf("mergeMembership(%v, %v, %v) = %v, want %v",
					tc.current, tc.managed, tc.desired, got, tc.want)
			}
		})
	}
}

// An outpost has to be reconciled the moment a provider naming it finishes
// registering. Without the watch it would sit out the requeue timer instead,
// which is the difference between an application coming back in a second and
// coming back in half a minute.
func TestOutpostsAreWokenByTheProvidersThatNameThem(t *testing.T) {
	scheme := serviceConnectionScheme(t)
	r := &OutpostReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	provider := memberProvider("team-a", "grafana", ptr32(7), "edge")
	requests := r.outpostsForProvider()(context.Background(), provider)

	want := []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: "team-a", Name: "edge"}}}
	if !slices.Equal(requests, want) {
		t.Errorf("requests = %v, want exactly %v", requests, want)
	}

	// Anything that does not carry outpost references maps to nothing rather
	// than panicking; the handler is given whatever the cache holds.
	if got := r.outpostsForProvider()(context.Background(), &corev1.Secret{}); got != nil {
		t.Errorf("requests for a Secret = %v, want none", got)
	}
}

// The index has to key on the same value desiredProviderIDs looks up, including
// the namespace default, or a provider is never collected.
func TestIndexProviderOutpostRefsKeysOnNamespaceAndName(t *testing.T) {
	provider := memberProvider("team-a", "grafana", ptr32(7), "edge")
	provider.Spec.OutpostRefs = append(provider.Spec.OutpostRefs,
		authentikv1alpha1.OutpostReference{Name: "shared", Namespace: "platform"})

	got := indexProviderOutpostRefs(provider)
	want := []string{"team-a/edge", "platform/shared"}
	if !slices.Equal(got, want) {
		t.Errorf("index keys = %v, want %v", got, want)
	}

	// Anything without outpost references indexes to nothing rather than
	// panicking.
	if keys := indexProviderOutpostRefs(&corev1.Secret{}); keys != nil {
		t.Errorf("index keys for a Secret = %v, want none", keys)
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
	outpost := newOutpost()
	r := &OutpostReconciler{}
	adapter := &outpostAdapter{
		client:             &stubAuthentikClient{},
		outpost:            outpost,
		desiredProviderIDs: []int32{7},
		// 9 is attached in authentik but was not attached by this operator.
		observed: &api.Outpost{Pk: "uuid", TokenIdentifier: "ak-outpost-edge", Providers: []int32{9}},
	}

	r.recordIdentity(outpost, SyncOutcome{RemoteID: "uuid"}, adapter, []int32{7}, "sc-uuid")

	if outpost.Status.OutpostID != "uuid" || outpost.Status.RemoteID != "uuid" {
		t.Errorf("status = %+v, want the UUID recorded", outpost.Status)
	}
	// Status reports what is attached, including the provider somebody else
	// attached, and separately what this operator put there. The second is what
	// the next reconcile diffs against to detach precisely.
	if !slices.Equal(outpost.Status.ProviderIDs, []int32{7, 9}) {
		t.Errorf("status.providerIDs = %v, want everything attached", outpost.Status.ProviderIDs)
	}
	if !slices.Equal(outpost.Status.ManagedProviderIDs, []int32{7}) {
		t.Errorf("status.managedProviderIDs = %v, want only what the operator attached",
			outpost.Status.ManagedProviderIDs)
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

	got, err := r.resolveServiceConnection(context.Background(), newOutpost(), testAuthentikClient())
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

	got, err := r.resolveServiceConnection(context.Background(), outpost, testAuthentikClient())
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

	_, err := r.resolveServiceConnection(context.Background(), outpost, testAuthentikClient())
	if err == nil {
		t.Fatal("expected an unresolved reference to fail")
	}
	reason, requeue := ResultFor(err)
	if reason != authentikv1alpha1.ReasonReferenceNotFound || !requeue {
		t.Errorf("ResultFor = (%s, %v), want (ReferenceNotFound, true)", reason, requeue)
	}
}

// A service connection that already exists in authentik is resolved by name,
// because there is no Kubernetes resource to read an ID from. A cluster may
// well have had service connections set up long before this operator arrived.
func TestResolveServiceConnectionByExistingName(t *testing.T) {
	r := &OutpostReconciler{}
	outpost := newOutpost()
	outpost.Spec.ServiceConnectionRef = &authentikv1alpha1.ServiceConnectionReference{
		ExistingServiceConnectionName: "hand-made-cluster",
	}

	ak := &stubAuthentikClient{
		baseURL:            "https://authentik.example",
		serviceConnections: map[string]string{"hand-made-cluster": "sc-uuid"},
	}

	got, err := r.resolveServiceConnection(context.Background(), outpost, ak)
	if err != nil {
		t.Fatalf("resolveServiceConnection: %v", err)
	}
	if got != "sc-uuid" {
		t.Errorf("uuid = %q, want the one authentik reports", got)
	}

	outpost.Spec.ServiceConnectionRef.ExistingServiceConnectionName = "absent"
	if _, err := r.resolveServiceConnection(context.Background(), outpost, ak); !authentik.IsNotFound(err) {
		t.Fatalf("err = %v, want a not-found naming the authentik object", err)
	}
}

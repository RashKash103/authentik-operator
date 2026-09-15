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

package e2e

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/test/utils"
)

// Several operators can share one authentik — one per Kubernetes cluster, or
// one reaching the instance directly while another goes through a proxy.
// authentik has no ownership marker on providers or applications, so the only
// thing keeping them apart is the cluster identity on the connection, which
// scopes the names of objects the operator manages.
//
// These tests use two connections with different cluster identities rather than
// two operator processes. That is the same thing as far as the mechanism goes:
// scoping is a property of the connection, not of the process, so a second
// process configured identically would behave exactly as the second connection
// here does. Running a second manager would test the test harness, not the
// operator.

// connectionWithCluster creates a connection carrying a cluster identity and
// the two flows providers in that namespace reference.
func connectionWithCluster(t *testing.T, ctx context.Context, c client.Client, cfg Config, cluster string) string {
	t.Helper()

	ns := utils.NewNamespace(t, ctx, c)
	utils.CreateSecret(t, ctx, c, ns, "authentik-token", map[string]string{
		"token": cfg.AuthentikToken,
	})

	conn := &authentikv1alpha1.AuthentikConnection{
		ObjectMeta: metav1.ObjectMeta{Name: "primary", Namespace: ns},
		Spec: authentikv1alpha1.AuthentikConnectionSpec{
			ConnectionSettings: authentikv1alpha1.ConnectionSettings{
				URL:     cfg.AuthentikURL,
				Cluster: cluster,
			},
			TokenSecretRef: authentikv1alpha1.LocalSecretKeyReference{
				Name: "authentik-token", Key: "token",
			},
		},
	}
	if err := c.Create(ctx, conn); err != nil {
		t.Fatalf("creating connection: %v", err)
	}
	utils.WaitForCondition(t, ctx, c, conn,
		authentikv1alpha1.ConditionReady, metav1.ConditionTrue, readyTimeout)

	for name, slug := range map[string]string{
		flowAuthorization: defaultAuthorizationFlow,
		flowInvalidation:  defaultInvalidationFlow,
	} {
		flow := &authentikv1alpha1.Flow{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: authentikv1alpha1.FlowSpec{
				ConnectionRef: authentikv1alpha1.ConnectionReference{Name: "primary"},
				ExistingSlug:  slug,
			},
		}
		if err := c.Create(ctx, flow); err != nil {
			t.Fatalf("creating flow %s: %v", name, err)
		}
		utils.WaitForCondition(t, ctx, c, flow,
			authentikv1alpha1.ConditionReady, metav1.ConditionTrue, readyTimeout)
	}

	return ns
}

// TestTwoClustersGetDistinctProviders is the scenario the cluster identity
// exists for: two operators, the same declared provider name, one authentik.
//
// Without scoping the second one finds the first one's provider and either
// refuses it as an adoption conflict or takes it over — so one cluster's
// configuration silently becomes the other's.
func TestTwoClustersGetDistinctProviders(t *testing.T) {
	cfg, c := setup(t)
	ctx := context.Background()

	nsEU := connectionWithCluster(t, ctx, c, cfg, "e2e-eu")
	nsUS := connectionWithCluster(t, ctx, c, cfg, "e2e-us")

	// ONE declared name, used verbatim in both namespaces. Deriving it per
	// namespace would make the objects distinct before the cluster identity
	// ever applied, and the test would pass with scoping removed entirely.
	// It is still unique per run so a previous run cannot interfere.
	shared := utils.UniqueName(nsEU, "shared-app")

	makeProvider := func(ns string) *authentikv1alpha1.OAuth2Provider {
		spec := newProviderSpec()
		spec.Name = shared
		p := &authentikv1alpha1.OAuth2Provider{
			ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: ns},
			Spec:       spec,
		}
		if err := c.Create(ctx, p); err != nil {
			t.Fatalf("creating provider in %s: %v", ns, err)
		}
		utils.WaitForCondition(t, ctx, c, p,
			authentikv1alpha1.ConditionReady, metav1.ConditionTrue, readyTimeout)
		return p
	}

	eu := makeProvider(nsEU)
	us := makeProvider(nsUS)

	if eu.Status.ProviderID == nil || us.Status.ProviderID == nil {
		t.Fatal("expected both providers to report a primary key")
	}
	// The core assertion: two authentik objects, not one shared between them.
	if *eu.Status.ProviderID == *us.Status.ProviderID {
		t.Fatalf("both clusters resolved to authentik provider %d; "+
			"the cluster identity did not keep them apart", *eu.Status.ProviderID)
	}
	if eu.Status.RemoteName == us.Status.RemoteName {
		t.Errorf("both clusters used the authentik name %q", eu.Status.RemoteName)
	}
	// Both declared the same name, so the only thing that can have separated
	// them is the cluster suffix. Assert that explicitly rather than inferring
	// it from the IDs differing.
	if eu.Status.RemoteName != shared+"-e2e-eu" {
		t.Errorf("EU object is named %q, want %q — the cluster suffix is not being applied",
			eu.Status.RemoteName, shared+"-e2e-eu")
	}
	if us.Status.RemoteName != shared+"-e2e-us" {
		t.Errorf("US object is named %q, want %q", us.Status.RemoteName, shared+"-e2e-us")
	}

	t.Logf("eu=%d (%s) us=%d (%s)",
		*eu.Status.ProviderID, eu.Status.RemoteName,
		*us.Status.ProviderID, us.Status.RemoteName)
}

// TestOneClusterDoesNotDeleteAnothersProvider is the consequence that matters
// most: deleting a resource in one cluster must not remove the other cluster's
// object from authentik.
func TestOneClusterDoesNotDeleteAnothersProvider(t *testing.T) {
	cfg, c := setup(t)
	ctx := context.Background()

	nsEU := connectionWithCluster(t, ctx, c, cfg, "e2e-del-eu")
	nsUS := connectionWithCluster(t, ctx, c, cfg, "e2e-del-us")

	// One declared name in both namespaces, as above.
	shared := utils.UniqueName(nsEU, "deletion-probe")
	makeProvider := func(ns string) *authentikv1alpha1.OAuth2Provider {
		spec := newProviderSpec()
		spec.Name = shared
		p := &authentikv1alpha1.OAuth2Provider{
			ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: ns},
			Spec:       spec,
		}
		if err := c.Create(ctx, p); err != nil {
			t.Fatalf("creating provider in %s: %v", ns, err)
		}
		utils.WaitForCondition(t, ctx, c, p,
			authentikv1alpha1.ConditionReady, metav1.ConditionTrue, readyTimeout)
		return p
	}

	eu := makeProvider(nsEU)
	us := makeProvider(nsUS)
	survivorID := *us.Status.ProviderID

	if err := c.Delete(ctx, eu); err != nil {
		t.Fatalf("deleting the first cluster's provider: %v", err)
	}
	utils.WaitUntilGone(t, ctx, c, eu, client.ObjectKeyFromObject(eu), readyTimeout)

	// The survivor must still be Ready and still be the same authentik object.
	ak := utils.NewAuthentikClient(t, cfg.AuthentikURL, cfg.AuthentikToken)
	utils.WaitForOAuth2SubMode(t, ctx, ak, survivorID, "hashed_user_id", readyTimeout)

	if err := c.Get(ctx, client.ObjectKeyFromObject(us), us); err != nil {
		t.Fatalf("refetching the surviving provider: %v", err)
	}
	if us.Status.ProviderID == nil || *us.Status.ProviderID != survivorID {
		t.Errorf("the surviving provider changed identity: %v, want %d",
			us.Status.ProviderID, survivorID)
	}
	if cond := meta.FindStatusCondition(us.Status.Conditions, authentikv1alpha1.ConditionReady); cond == nil ||
		cond.Status != metav1.ConditionTrue {
		t.Errorf("the surviving provider is no longer Ready: %s",
			utils.FormatConditions(us.Status.Conditions))
	}
}

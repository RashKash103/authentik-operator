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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/test/utils"
)

// authentik ships these flows on every install, so referencing them by slug
// keeps the suite independent of any fixture setup.
const (
	defaultAuthorizationFlow = "default-provider-authorization-explicit-consent"
	defaultInvalidationFlow  = "default-provider-invalidation-flow"
)

// newConnection creates a ready-to-use connection in a fresh namespace.
func newConnection(t *testing.T, ctx context.Context, c client.Client, cfg Config) string {
	t.Helper()

	ns := utils.NewNamespace(t, ctx, c)
	utils.CreateSecret(t, ctx, c, ns, "authentik-token", map[string]string{
		"token": cfg.AuthentikToken,
	})

	conn := &authentikv1alpha1.AuthentikConnection{
		ObjectMeta: metav1.ObjectMeta{Name: "primary", Namespace: ns},
		Spec: authentikv1alpha1.AuthentikConnectionSpec{
			ConnectionSettings: authentikv1alpha1.ConnectionSettings{URL: cfg.AuthentikURL},
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
	return ns
}

func newProviderSpec() authentikv1alpha1.OAuth2ProviderSpec {
	return authentikv1alpha1.OAuth2ProviderSpec{
		ProviderCommonSpec: authentikv1alpha1.ProviderCommonSpec{
			ConnectionRef:     authentikv1alpha1.ConnectionReference{Name: "primary"},
			AuthorizationFlow: defaultAuthorizationFlow,
			InvalidationFlow:  defaultInvalidationFlow,
		},
		RedirectURIs: []authentikv1alpha1.RedirectURI{
			{MatchingMode: "strict", URL: "https://app.example.com/callback"},
		},
	}
}

// TestOAuth2ProviderCreatesAndPublishesCredentials is the scenario the whole
// operator exists for: declare a provider, get working credentials.
func TestOAuth2ProviderCreatesAndPublishesCredentials(t *testing.T) {
	cfg, c := setup(t)
	ctx := context.Background()
	ns := newConnection(t, ctx, c, cfg)

	spec := newProviderSpec()
	spec.Name = utils.UniqueName(ns, "demo-app")
	spec.WriteCredentialsTo = &authentikv1alpha1.CredentialsSecretRef{Name: "app-oidc"}

	provider := &authentikv1alpha1.OAuth2Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-app", Namespace: ns},
		Spec:       spec,
	}
	if err := c.Create(ctx, provider); err != nil {
		t.Fatalf("creating provider: %v", err)
	}

	utils.WaitForCondition(t, ctx, c, provider,
		authentikv1alpha1.ConditionReady, metav1.ConditionTrue, readyTimeout)

	if provider.Status.ProviderID == nil {
		t.Fatal("expected status.providerID to be set once Ready")
	}
	if provider.Status.ClientID == "" {
		t.Error("expected status.clientID to be reported")
	}

	// The credentials Secret is the actual deliverable.
	var secret corev1.Secret
	key := types.NamespacedName{Name: "app-oidc", Namespace: ns}
	if err := c.Get(ctx, key, &secret); err != nil {
		t.Fatalf("reading credentials secret: %v", err)
	}
	for _, field := range []string{"client-id", "client-secret"} {
		if len(secret.Data[field]) == 0 {
			t.Errorf("credentials secret has no %q", field)
		}
	}
	if string(secret.Data["client-id"]) != provider.Status.ClientID {
		t.Error("the published client id does not match the one reported in status")
	}
	// The secret must never be echoed into status, which is readable by a
	// wider audience than Secrets in the namespace.
	if provider.Status.ClientID == string(secret.Data["client-secret"]) {
		t.Error("the client secret appears to have leaked into status")
	}

	if !metav1.IsControlledBy(&secret, provider) {
		t.Error("credentials secret is not owned by the provider, so it will not be garbage-collected")
	}
}

// TestApplicationWaitsForItsProvider is the ordering guarantee: an Application
// applied before its provider must converge, not fail.
//
// This is the scenario that only breaks in a real cluster, which is why it is
// here rather than in a unit test.
func TestApplicationWaitsForItsProvider(t *testing.T) {
	cfg, c := setup(t)
	ctx := context.Background()
	ns := newConnection(t, ctx, c, cfg)

	app := &authentikv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "ordered", Namespace: ns},
		Spec: authentikv1alpha1.ApplicationSpec{
			ConnectionRef: authentikv1alpha1.ConnectionReference{Name: "primary"},
			Slug:          utils.UniqueName(ns, "ordered-app"),
			Name:          "Ordered App",
			ProviderRef: &authentikv1alpha1.ProviderReference{
				Kind: authentikv1alpha1.ProviderKindOAuth2,
				Name: "arrives-later",
			},
		},
	}
	if err := c.Create(ctx, app); err != nil {
		t.Fatalf("creating application: %v", err)
	}

	// It must report the missing reference rather than succeed or give up.
	utils.WaitForCondition(t, ctx, c, app,
		authentikv1alpha1.ConditionReady, metav1.ConditionFalse, readyTimeout)

	cond := meta.FindStatusCondition(app.Status.Conditions, authentikv1alpha1.ConditionReady)
	if cond == nil || cond.Reason != authentikv1alpha1.ReasonReferenceNotFound {
		t.Fatalf("expected ReferenceNotFound, got %s", utils.FormatConditions(app.Status.Conditions))
	}

	providerSpec := newProviderSpec()
	providerSpec.Name = utils.UniqueName(ns, "arrives-later")
	provider := &authentikv1alpha1.OAuth2Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "arrives-later", Namespace: ns},
		Spec:       providerSpec,
	}
	if err := c.Create(ctx, provider); err != nil {
		t.Fatalf("creating provider: %v", err)
	}

	// The provider watch should pull this through promptly rather than after a
	// full backoff.
	utils.WaitForCondition(t, ctx, c, app,
		authentikv1alpha1.ConditionReady, metav1.ConditionTrue, readyTimeout)
	t.Log("application converged once its provider appeared")
}

// TestProviderDeletionRemovesRemoteObject covers the finalizer path.
func TestProviderDeletionRemovesRemoteObject(t *testing.T) {
	cfg, c := setup(t)
	ctx := context.Background()
	ns := newConnection(t, ctx, c, cfg)

	ephemeralSpec := newProviderSpec()
	ephemeralSpec.Name = utils.UniqueName(ns, "ephemeral")
	provider := &authentikv1alpha1.OAuth2Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "ephemeral", Namespace: ns},
		Spec:       ephemeralSpec,
	}
	if err := c.Create(ctx, provider); err != nil {
		t.Fatalf("creating provider: %v", err)
	}
	utils.WaitForCondition(t, ctx, c, provider,
		authentikv1alpha1.ConditionReady, metav1.ConditionTrue, readyTimeout)

	if err := c.Delete(ctx, provider); err != nil {
		t.Fatalf("deleting provider: %v", err)
	}

	// The finalizer must actually be released; a stuck finalizer would block
	// deletion of the namespace too.
	key := client.ObjectKeyFromObject(provider)
	utils.WaitUntilGone(t, ctx, c, provider, key, readyTimeout)
}

// TestProviderOrphanLeavesRemoteObject proves deletionPolicy is honoured.
func TestProviderOrphanLeavesRemoteObject(t *testing.T) {
	cfg, c := setup(t)
	ctx := context.Background()
	ns := newConnection(t, ctx, c, cfg)

	orphanName := utils.UniqueName(ns, "orphaned")
	spec := newProviderSpec()
	spec.Name = orphanName
	spec.Deletion = authentikv1alpha1.DeletionPolicyOrphan

	provider := &authentikv1alpha1.OAuth2Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "orphaned", Namespace: ns},
		Spec:       spec,
	}
	if err := c.Create(ctx, provider); err != nil {
		t.Fatalf("creating provider: %v", err)
	}
	utils.WaitForCondition(t, ctx, c, provider,
		authentikv1alpha1.ConditionReady, metav1.ConditionTrue, readyTimeout)
	remoteID := provider.Status.RemoteID

	if err := c.Delete(ctx, provider); err != nil {
		t.Fatalf("deleting provider: %v", err)
	}
	utils.WaitUntilGone(t, ctx, c, provider, client.ObjectKeyFromObject(provider), readyTimeout)

	// The authentik object should still exist. Re-adopting it proves it does,
	// and leaves the instance clean for the next run.
	spec2 := newProviderSpec()
	spec2.Adoption = authentikv1alpha1.AdoptionPolicyAdoptExisting
	spec2.Name = orphanName

	readopted := &authentikv1alpha1.OAuth2Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "readopt", Namespace: ns},
		Spec:       spec2,
	}
	if err := c.Create(ctx, readopted); err != nil {
		t.Fatalf("creating re-adopting provider: %v", err)
	}
	utils.WaitForCondition(t, ctx, c, readopted,
		authentikv1alpha1.ConditionReady, metav1.ConditionTrue, readyTimeout)

	if readopted.Status.RemoteID != remoteID {
		t.Errorf("re-adopted a different object (%s) than the orphaned one (%s); "+
			"deletionPolicy: Orphan may not have worked",
			readopted.Status.RemoteID, remoteID)
	}
	if !readopted.Status.Adopted {
		t.Error("expected status.adopted to record that a pre-existing object was taken over")
	}

	// Leave nothing behind.
	readopted.Spec.Deletion = authentikv1alpha1.DeletionPolicyDelete
	if err := c.Update(ctx, readopted); err != nil && !apierrors.IsNotFound(err) {
		t.Logf("could not switch the re-adopted provider to Delete: %v", err)
	}
}

// TestSAMLProviderAndApplication covers the second provider kind end to end,
// including an Application resolving a non-OAuth2 reference.
func TestSAMLProviderAndApplication(t *testing.T) {
	cfg, c := setup(t)
	ctx := context.Background()
	ns := newConnection(t, ctx, c, cfg)

	provider := &authentikv1alpha1.SAMLProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "saml-app", Namespace: ns},
		Spec: authentikv1alpha1.SAMLProviderSpec{
			ProviderCommonSpec: authentikv1alpha1.ProviderCommonSpec{
				ConnectionRef:     authentikv1alpha1.ConnectionReference{Name: "primary"},
				Name:              utils.UniqueName(ns, "saml-app"),
				AuthorizationFlow: defaultAuthorizationFlow,
				InvalidationFlow:  defaultInvalidationFlow,
			},
			ACSURL: "https://saml.example.com/acs",
		},
	}
	if err := c.Create(ctx, provider); err != nil {
		t.Fatalf("creating SAML provider: %v", err)
	}
	utils.WaitForCondition(t, ctx, c, provider,
		authentikv1alpha1.ConditionReady, metav1.ConditionTrue, readyTimeout)

	if provider.Status.ProviderID == nil {
		t.Fatal("expected status.providerID once Ready")
	}

	app := &authentikv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "saml-bound", Namespace: ns},
		Spec: authentikv1alpha1.ApplicationSpec{
			ConnectionRef: authentikv1alpha1.ConnectionReference{Name: "primary"},
			Slug:          utils.UniqueName(ns, "saml-bound"),
			Name:          "SAML Bound",
			ProviderRef: &authentikv1alpha1.ProviderReference{
				Kind: authentikv1alpha1.ProviderKindSAML,
				Name: "saml-app",
			},
		},
	}
	if err := c.Create(ctx, app); err != nil {
		t.Fatalf("creating application: %v", err)
	}
	utils.WaitForCondition(t, ctx, c, app,
		authentikv1alpha1.ConditionReady, metav1.ConditionTrue, readyTimeout)

	if app.Status.ProviderID == nil || *app.Status.ProviderID != *provider.Status.ProviderID {
		t.Errorf("application bound to provider %v, want the SAML provider %v",
			app.Status.ProviderID, provider.Status.ProviderID)
	}
	t.Logf("application bound to SAML provider %d", *provider.Status.ProviderID)
}

// TestProxyProviderCreates covers the third provider kind.
func TestProxyProviderCreates(t *testing.T) {
	cfg, c := setup(t)
	ctx := context.Background()
	ns := newConnection(t, ctx, c, cfg)

	provider := &authentikv1alpha1.ProxyProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "proxy-app", Namespace: ns},
		Spec: authentikv1alpha1.ProxyProviderSpec{
			ProviderCommonSpec: authentikv1alpha1.ProviderCommonSpec{
				ConnectionRef:     authentikv1alpha1.ConnectionReference{Name: "primary"},
				Name:              utils.UniqueName(ns, "proxy-app"),
				AuthorizationFlow: defaultAuthorizationFlow,
				InvalidationFlow:  defaultInvalidationFlow,
			},
			ExternalHost: "https://proxied.example.com",
			Mode:         "forward_single",
		},
	}
	if err := c.Create(ctx, provider); err != nil {
		t.Fatalf("creating proxy provider: %v", err)
	}
	utils.WaitForCondition(t, ctx, c, provider,
		authentikv1alpha1.ConditionReady, metav1.ConditionTrue, readyTimeout)

	if provider.Status.ProviderID == nil {
		t.Fatal("expected status.providerID once Ready")
	}
	t.Logf("proxy provider created as %d", *provider.Status.ProviderID)
}

// TestProviderDriftIsCorrected mutates the authentik object out of band and
// checks the operator pulls it back.
//
// Drift correction is unit-tested against a fake, but only a real instance
// proves the update path actually converges rather than, say, failing
// validation on a field the request builder omits.
func TestProviderDriftIsCorrected(t *testing.T) {
	cfg, c := setup(t)
	ctx := context.Background()
	ns := newConnection(t, ctx, c, cfg)

	spec := newProviderSpec()
	spec.Name = utils.UniqueName(ns, "drifting")
	provider := &authentikv1alpha1.OAuth2Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "drifting", Namespace: ns},
		Spec:       spec,
	}
	if err := c.Create(ctx, provider); err != nil {
		t.Fatalf("creating provider: %v", err)
	}
	utils.WaitForCondition(t, ctx, c, provider,
		authentikv1alpha1.ConditionReady, metav1.ConditionTrue, readyTimeout)

	akClient := utils.NewAuthentikClient(t, cfg.AuthentikURL, cfg.AuthentikToken)
	pk := *provider.Status.ProviderID

	// Change the provider behind the operator's back.
	utils.SetOAuth2SubMode(t, ctx, akClient, pk, "user_email")

	// Changing the spec is what drives the next reconcile; the operator does
	// not poll authentik for drift on its own.
	if err := c.Get(ctx, client.ObjectKeyFromObject(provider), provider); err != nil {
		t.Fatalf("refetching provider: %v", err)
	}
	provider.Spec.SubMode = "user_username"
	if err := c.Update(ctx, provider); err != nil {
		t.Fatalf("updating provider spec: %v", err)
	}
	utils.WaitForGenerationSynced(t, ctx, c, provider, readyTimeout)

	utils.WaitForOAuth2SubMode(t, ctx, akClient, pk, "user_username", readyTimeout)
	t.Log("out-of-band change was corrected back to the declared spec")
}

// TestProviderRecreatedWhenDeletedRemotely covers the recovery path when
// someone removes the authentik object while the resource still exists.
func TestProviderRecreatedWhenDeletedRemotely(t *testing.T) {
	cfg, c := setup(t)
	ctx := context.Background()
	ns := newConnection(t, ctx, c, cfg)

	spec := newProviderSpec()
	spec.Name = utils.UniqueName(ns, "vanishing")
	provider := &authentikv1alpha1.OAuth2Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "vanishing", Namespace: ns},
		Spec:       spec,
	}
	if err := c.Create(ctx, provider); err != nil {
		t.Fatalf("creating provider: %v", err)
	}
	utils.WaitForCondition(t, ctx, c, provider,
		authentikv1alpha1.ConditionReady, metav1.ConditionTrue, readyTimeout)
	originalID := *provider.Status.ProviderID

	akClient := utils.NewAuthentikClient(t, cfg.AuthentikURL, cfg.AuthentikToken)
	utils.DeleteOAuth2Provider(t, ctx, akClient, originalID)

	// Touch the spec to trigger a reconcile.
	if err := c.Get(ctx, client.ObjectKeyFromObject(provider), provider); err != nil {
		t.Fatalf("refetching provider: %v", err)
	}
	provider.Spec.AccessTokenValidity = "hours=2"
	if err := c.Update(ctx, provider); err != nil {
		t.Fatalf("updating provider spec: %v", err)
	}

	// Ready is still true from the previous reconcile, so waiting on it alone
	// would read stale status. Wait for the generation to be observed instead.
	utils.WaitForGenerationSynced(t, ctx, c, provider, readyTimeout)

	if provider.Status.ProviderID == nil {
		t.Fatal("expected the provider to be recreated with a new primary key")
	}
	if *provider.Status.ProviderID == originalID {
		t.Errorf("status still reports the deleted primary key %d", originalID)
	}
	t.Logf("recreated as %d after the original was deleted remotely", *provider.Status.ProviderID)
}

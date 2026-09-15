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

// Package e2e exercises the operator against a real Kubernetes cluster and a
// real authentik instance.
//
// # Running these tests
//
// They are opt-in and skip by default. That is deliberate: the suite creates
// namespaces and custom resources, and a developer's current kubecontext is
// very often a real cluster they did not intend to point a test suite at.
// Requiring an explicit opt-in means `go test ./...` can never accidentally
// write to production.
//
//	just authentik-up
//	export E2E_ENABLED=true
//	export E2E_KUBECONTEXT=kind-authentik-operator-e2e
//	export AUTHENTIK_URL=http://localhost:9000
//	export AUTHENTIK_TOKEN=authentik-operator-e2e-bootstrap-token
//	just test-e2e
package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/test/utils"
)

// Environment variables controlling the suite.
const (
	envEnabled        = "E2E_ENABLED"
	envKubeContext    = "E2E_KUBECONTEXT"
	envAuthentikURL   = "AUTHENTIK_URL"
	envAuthentikToken = "AUTHENTIK_TOKEN" //nolint:gosec // the name of a variable, not a credential
)

// readyTimeout is generous: the first reconcile may wait on the operator's
// leader election lease.
const readyTimeout = 3 * time.Minute

// Config is the resolved environment for a run.
type Config struct {
	KubeContext    string
	AuthentikURL   string
	AuthentikToken string
}

// enabled reports whether the suite has been explicitly switched on.
//
// It is a separate function purely so the opt-in guard itself is testable
// without the test having to skip itself.
func enabled() bool {
	return os.Getenv(envEnabled) == "true"
}

// setup skips or fails fast, then returns a client bound to the named context.
func setup(t *testing.T) (Config, client.Client) {
	t.Helper()

	if !enabled() {
		t.Skipf("set %s=true to run the end-to-end suite", envEnabled)
	}

	cfg := Config{
		KubeContext:    os.Getenv(envKubeContext),
		AuthentikURL:   os.Getenv(envAuthentikURL),
		AuthentikToken: os.Getenv(envAuthentikToken),
	}

	var missing []string
	for name, value := range map[string]string{
		envKubeContext:    cfg.KubeContext,
		envAuthentikURL:   cfg.AuthentikURL,
		envAuthentikToken: cfg.AuthentikToken,
	} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		// Fail rather than skip: the operator asked for the suite to run, so
		// silently skipping would report a green run that tested nothing.
		t.Fatalf("%s=true but these are unset: %v", envEnabled, missing)
	}

	c, err := utils.NewClient(cfg.KubeContext)
	if err != nil {
		t.Fatalf("building cluster client: %v", err)
	}
	return cfg, c
}

// TestSuiteIsOptIn enforces the safety property described in the package
// comment: absent an explicit opt-in, nothing here touches a cluster.
func TestSuiteIsOptIn(t *testing.T) {
	t.Setenv(envEnabled, "")
	if enabled() {
		t.Error("suite reported enabled with the opt-in variable unset")
	}

	t.Setenv(envEnabled, "1")
	if enabled() {
		t.Error(`only the exact value "true" should enable the suite, not "1"`)
	}

	t.Setenv(envEnabled, "true")
	if !enabled() {
		t.Error(`suite did not report enabled with the variable set to "true"`)
	}
}

// TestConnectionBecomesReady is the core scenario: a connection pointed at a
// live authentik reports Ready together with the version it detected.
func TestConnectionBecomesReady(t *testing.T) {
	cfg, c := setup(t)
	ctx := context.Background()

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
		t.Fatalf("creating AuthentikConnection: %v", err)
	}

	utils.WaitForCondition(t, ctx, c, conn,
		authentikv1alpha1.ConditionReady, metav1.ConditionTrue, readyTimeout)

	if conn.Status.AuthentikVersion == "" {
		t.Error("expected status.authentikVersion to be populated once Ready")
	}
	if conn.Status.VersionSupported == nil || !*conn.Status.VersionSupported {
		t.Errorf("version %q reported unsupported, but the test stack is pinned to a supported series",
			conn.Status.AuthentikVersion)
	}
	t.Logf("connection Ready against authentik %s", conn.Status.AuthentikVersion)
}

// TestConnectionWithBadTokenIsNotReady proves the failure path reports
// something actionable rather than hanging or going Ready regardless.
func TestConnectionWithBadTokenIsNotReady(t *testing.T) {
	cfg, c := setup(t)
	ctx := context.Background()

	ns := utils.NewNamespace(t, ctx, c)
	utils.CreateSecret(t, ctx, c, ns, "bad-token", map[string]string{
		"token": "definitely-not-a-valid-token",
	})

	conn := &authentikv1alpha1.AuthentikConnection{
		ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: ns},
		Spec: authentikv1alpha1.AuthentikConnectionSpec{
			ConnectionSettings: authentikv1alpha1.ConnectionSettings{URL: cfg.AuthentikURL},
			TokenSecretRef: authentikv1alpha1.LocalSecretKeyReference{
				Name: "bad-token", Key: "token",
			},
		},
	}
	if err := c.Create(ctx, conn); err != nil {
		t.Fatalf("creating AuthentikConnection: %v", err)
	}

	utils.WaitForCondition(t, ctx, c, conn,
		authentikv1alpha1.ConditionReady, metav1.ConditionFalse, readyTimeout)

	cond := meta.FindStatusCondition(conn.Status.Conditions, authentikv1alpha1.ConditionReady)
	if cond == nil || cond.Message == "" {
		t.Fatalf("expected a Ready condition carrying a message, got %s",
			utils.FormatConditions(conn.Status.Conditions))
	}
	t.Logf("rejected as expected: %s: %s", cond.Reason, cond.Message)
}

// TestConnectionRecoversFromMissingSecret covers the rotation path: a
// connection whose Secret is absent must recover once it appears, without
// restarting the operator.
func TestConnectionRecoversFromMissingSecret(t *testing.T) {
	cfg, c := setup(t)
	ctx := context.Background()

	ns := utils.NewNamespace(t, ctx, c)

	conn := &authentikv1alpha1.AuthentikConnection{
		ObjectMeta: metav1.ObjectMeta{Name: "late-secret", Namespace: ns},
		Spec: authentikv1alpha1.AuthentikConnectionSpec{
			ConnectionSettings: authentikv1alpha1.ConnectionSettings{URL: cfg.AuthentikURL},
			TokenSecretRef: authentikv1alpha1.LocalSecretKeyReference{
				Name: "arrives-later", Key: "token",
			},
		},
	}
	if err := c.Create(ctx, conn); err != nil {
		t.Fatalf("creating AuthentikConnection: %v", err)
	}

	utils.WaitForCondition(t, ctx, c, conn,
		authentikv1alpha1.ConditionReady, metav1.ConditionFalse, readyTimeout)

	utils.CreateSecret(t, ctx, c, ns, "arrives-later", map[string]string{
		"token": cfg.AuthentikToken,
	})

	// The Secret watch should pick this up promptly rather than waiting out
	// the probe interval.
	utils.WaitForCondition(t, ctx, c, conn,
		authentikv1alpha1.ConditionReady, metav1.ConditionTrue, readyTimeout)
	t.Logf("recovered once the Secret appeared, against authentik %s", conn.Status.AuthentikVersion)
}

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

// Package utils provides the harness the end-to-end suite runs on.
package utils

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
)

// NewClient builds a client bound to an explicitly named kubecontext.
//
// The context is never defaulted to whatever is current. This suite creates
// namespaces and custom resources, and a developer's current context is very
// often a real cluster; silently inheriting it is the one failure mode worth
// designing out entirely.
func NewClient(kubeContext string) (client.Client, error) {
	if strings.TrimSpace(kubeContext) == "" {
		return nil, fmt.Errorf("a kubecontext must be named explicitly; refusing to use the current context")
	}

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{CurrentContext: kubeContext}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("building client config for context %q: %w", kubeContext, err)
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := authentikv1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	return client.New(cfg, client.Options{Scheme: scheme})
}

// NewNamespace creates a uniquely named namespace and removes it when the test
// finishes.
//
// Names are unique per test so the suite can run in parallel against one
// cluster without collisions, and cleanup is registered immediately so a test
// that fails halfway still tidies up.
func NewNamespace(t *testing.T, ctx context.Context, c client.Client) string {
	t.Helper()

	name := fmt.Sprintf("e2e-%s", utilrand.String(8))
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := c.Create(ctx, ns); err != nil {
		t.Fatalf("creating namespace %s: %v", name, err)
	}

	t.Cleanup(func() {
		// A fresh context: the test's own may already be cancelled, and
		// skipping cleanup would leak namespaces across runs.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := c.Delete(cleanupCtx, ns); err != nil && !apierrors.IsNotFound(err) {
			t.Logf("failed to delete namespace %s: %v", name, err)
		}
	})

	return name
}

// UniqueName derives a name unique to one test run from its namespace.
//
// Namespaces are per-test, but authentik objects are named from the spec and
// live in an instance that outlives the run. Without a per-run suffix the
// second run of the suite collides with the first and fails with an adoption
// conflict - which is what happened before this existed.
func UniqueName(namespace, base string) string {
	suffix := namespace
	if idx := strings.LastIndex(namespace, "-"); idx >= 0 {
		suffix = namespace[idx+1:]
	}
	return base + "-" + suffix
}

// CreateSecret creates an opaque Secret in the given namespace.
func CreateSecret(t *testing.T, ctx context.Context, c client.Client, namespace, name string, data map[string]string) {
	t.Helper()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		StringData: data,
		Type:       corev1.SecretTypeOpaque,
	}
	if err := c.Create(ctx, secret); err != nil {
		t.Fatalf("creating secret %s/%s: %v", namespace, name, err)
	}
}

// WaitForCondition polls until obj reports condType with the wanted status.
//
// On timeout it reports the conditions actually observed, because "timed out
// waiting for Ready" on its own tells you nothing about why.
func WaitForCondition(
	t *testing.T,
	ctx context.Context,
	c client.Client,
	obj client.Object,
	condType string,
	want metav1.ConditionStatus,
	timeout time.Duration,
) {
	t.Helper()

	key := client.ObjectKeyFromObject(obj)
	deadline := time.Now().Add(timeout)
	var last []metav1.Condition

	for time.Now().Before(deadline) {
		if err := c.Get(ctx, key, obj); err != nil {
			if !apierrors.IsNotFound(err) {
				t.Fatalf("getting %s: %v", key, err)
			}
		} else {
			conds, err := conditionsOf(obj)
			if err != nil {
				t.Fatalf("reading conditions of %s: %v", key, err)
			}
			last = conds
			if cond := meta.FindStatusCondition(conds, condType); cond != nil && cond.Status == want {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatalf("timed out after %s waiting for %s on %s to be %s; observed conditions: %s",
		timeout, condType, key, want, FormatConditions(last))
}

// FormatConditions renders conditions compactly for a failure message.
func FormatConditions(conds []metav1.Condition) string {
	if len(conds) == 0 {
		return "(none reported)"
	}
	parts := make([]string, 0, len(conds))
	for _, c := range conds {
		parts = append(parts, fmt.Sprintf("%s=%s(%s: %s)", c.Type, c.Status, c.Reason, c.Message))
	}
	return strings.Join(parts, ", ")
}

// conditionsOf extracts conditions from any of the operator's resource types.
func conditionsOf(obj client.Object) ([]metav1.Condition, error) {
	switch o := obj.(type) {
	case *authentikv1alpha1.AuthentikConnection:
		return o.Status.Conditions, nil
	case *authentikv1alpha1.ClusterAuthentikConnection:
		return o.Status.Conditions, nil
	case *authentikv1alpha1.OAuth2Provider:
		return o.Status.Conditions, nil
	case *authentikv1alpha1.Application:
		return o.Status.Conditions, nil
	case *authentikv1alpha1.SAMLProvider:
		return o.Status.Conditions, nil
	case *authentikv1alpha1.ProxyProvider:
		return o.Status.Conditions, nil
	default:
		return nil, fmt.Errorf("no condition accessor for %T", obj)
	}
}

// WaitUntilGone polls until the object no longer exists.
//
// A resource that lingers is usually a finalizer the operator failed to
// release, which would also block deletion of its namespace, so the failure
// message reports the finalizers still present.
func WaitUntilGone(
	t *testing.T,
	ctx context.Context,
	c client.Client,
	obj client.Object,
	key client.ObjectKey,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		err := c.Get(ctx, key, obj)
		if apierrors.IsNotFound(err) {
			return
		}
		if err != nil {
			t.Fatalf("getting %s: %v", key, err)
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatalf("timed out after %s waiting for %s to be deleted; finalizers still set: %v",
		timeout, key, obj.GetFinalizers())
}

// WaitForGenerationSynced polls until the object's Ready condition reflects its
// current generation.
//
// Waiting on Ready alone is a race whenever the spec has just been edited: the
// condition is still true from the previous reconcile, so the wait returns
// immediately and the test reads stale status. Comparing the condition's
// observedGeneration against metadata.generation is what makes the wait mean
// "the operator has seen this version of the spec".
func WaitForGenerationSynced(
	t *testing.T,
	ctx context.Context,
	c client.Client,
	obj client.Object,
	timeout time.Duration,
) {
	t.Helper()

	key := client.ObjectKeyFromObject(obj)
	deadline := time.Now().Add(timeout)
	var last []metav1.Condition

	for time.Now().Before(deadline) {
		if err := c.Get(ctx, key, obj); err != nil {
			if !apierrors.IsNotFound(err) {
				t.Fatalf("getting %s: %v", key, err)
			}
		} else {
			conds, err := conditionsOf(obj)
			if err != nil {
				t.Fatalf("reading conditions of %s: %v", key, err)
			}
			last = conds
			cond := meta.FindStatusCondition(conds, "Ready")
			if cond != nil &&
				cond.Status == metav1.ConditionTrue &&
				cond.ObservedGeneration == obj.GetGeneration() {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatalf("timed out after %s waiting for %s to reconcile generation %d; observed: %s",
		timeout, key, obj.GetGeneration(), FormatConditions(last))
}

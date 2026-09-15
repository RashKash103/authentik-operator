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
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// Crossing namespaces is what the operator-level flag gates. A test that only
// covers the permitted direction would pass with the gate deleted.
func TestResolveNamespaceHonoursThePolicy(t *testing.T) {
	cases := []struct {
		name      string
		policy    ReferencePolicy
		own       string
		requested string
		want      string
		wantErr   bool
	}{
		{
			name: "an unset namespace stays local",
			own:  "team-a", requested: "", want: "team-a",
		},
		{
			// Naming your own namespace explicitly is not a cross-namespace
			// reference, so it must work with the gate closed.
			name: "naming the own namespace is not crossing",
			own:  "team-a", requested: "team-a", want: "team-a",
		},
		{
			name: "crossing is refused by default",
			own:  "team-a", requested: "shared", wantErr: true,
		},
		{
			name:   "crossing is permitted once enabled",
			policy: ReferencePolicy{AllowCrossNamespace: true},
			own:    "team-a", requested: "shared", want: "shared",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveNamespace(tc.policy, "spec.authorizationFlow", tc.own, tc.requested)
			if tc.wantErr {
				if !errors.Is(err, ErrCrossNamespaceDisabled) {
					t.Fatalf("err = %v, want ErrCrossNamespaceDisabled", err)
				}
				// Falling back to the local namespace would resolve a different
				// object than the manifest names, so nothing may come back.
				if got != "" {
					t.Errorf("namespace = %q, want no fallback on a refusal", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveNamespace: %v", err)
			}
			if got != tc.want {
				t.Errorf("namespace = %q, want %q", got, tc.want)
			}
		})
	}
}

// authentik UUIDs only mean something on the instance that issued them, so a
// reference reaching across instances has to be refused rather than forwarded.
func TestAssertSameInstance(t *testing.T) {
	const own = "https://authentik.example"

	if err := assertSameInstance("spec.authorizationFlow", "Flow", "default", own, own); err != nil {
		t.Errorf("the same instance must be accepted: %v", err)
	}
	// An empty URL on either side means the check has nothing to compare; unit
	// tests and resources reconciled before the URL was recorded rely on this.
	if err := assertSameInstance("spec.authorizationFlow", "Flow", "default", "", own); err != nil {
		t.Errorf("an unknown referenced instance must not fail: %v", err)
	}

	err := assertSameInstance("spec.authorizationFlow", "Flow", "default",
		"https://other.example", own)
	if !errors.Is(err, authentik.ErrValidation) {
		t.Fatalf("err = %v, want a validation error so the resource does not spin", err)
	}
	// A permanent error, because retrying resolves the same wrong instance.
	if _, retry := ResultFor(err); retry {
		t.Error("a cross-instance reference must not be retried")
	}
}

// The end-to-end path, through a real reference: a Flow in another namespace is
// invisible with the gate closed and resolves with it open.
func TestResolveFlowRefAcrossNamespaces(t *testing.T) {
	ctx := context.Background()
	scheme := serviceConnectionScheme(t)

	flow := &authentikv1alpha1.Flow{
		ObjectMeta: metav1.ObjectMeta{Name: "default-authz", Namespace: "shared"},
		Spec:       authentikv1alpha1.FlowSpec{ExistingSlug: "default-authorization-flow"},
	}
	flow.Status.RemoteID = "flow-uuid"
	flow.Status.AuthentikURL = "https://authentik.example"

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(flow).Build()
	ref := authentikv1alpha1.FlowReference{Name: "default-authz", Namespace: "shared"}

	closed := ReferenceScope{Namespace: "team-a", AuthentikURL: "https://authentik.example"}
	if _, err := resolveFlowRef(ctx, c, closed, "spec.authorizationFlow", ref); !errors.Is(err, ErrCrossNamespaceDisabled) {
		t.Fatalf("err = %v, want the reference refused by policy", err)
	}

	open := closed
	open.Policy.AllowCrossNamespace = true
	got, err := resolveFlowRef(ctx, c, open, "spec.authorizationFlow", ref)
	if err != nil {
		t.Fatalf("resolveFlowRef: %v", err)
	}
	if got != "flow-uuid" {
		t.Errorf("uuid = %q, want the referenced flow's remoteID", got)
	}

	// Same reference, different authentik: permitted by namespace policy but
	// still refused, because the UUID would be meaningless to that instance.
	elsewhere := open
	elsewhere.AuthentikURL = "https://other.example"
	if _, err := resolveFlowRef(ctx, c, elsewhere, "spec.authorizationFlow", ref); !errors.Is(err, authentik.ErrValidation) {
		t.Fatalf("err = %v, want a cross-instance reference refused", err)
	}
}

// A reference to a resource that exists but has not reached authentik yet is a
// "not yet", not a "never" -- applying a provider before its Flow is ordinary.
func TestUnresolvedFlowReferenceRequeues(t *testing.T) {
	ctx := context.Background()
	scheme := serviceConnectionScheme(t)

	flow := &authentikv1alpha1.Flow{
		ObjectMeta: metav1.ObjectMeta{Name: "default-authz", Namespace: "team-a"},
		Spec:       authentikv1alpha1.FlowSpec{ExistingSlug: "default-authorization-flow"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(flow).Build()

	scope := ReferenceScope{Namespace: "team-a"}
	_, err := resolveFlowRef(ctx, c, scope, "spec.authorizationFlow",
		authentikv1alpha1.FlowReference{Name: "default-authz"})
	if !authentik.IsNotFound(err) {
		t.Fatalf("err = %v, want a not-found so the resource requeues", err)
	}
	if _, retry := ResultFor(err); !retry {
		t.Error("an unresolved reference must be retried")
	}
}

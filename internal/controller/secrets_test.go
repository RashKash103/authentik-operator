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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
)

// These cover the exploit the ownership guard exists to stop: writeCredentialsTo
// names any Secret in the namespace, so without a check, permission to create a
// provider becomes permission to overwrite and garbage-collect unrelated
// Secrets.

// secretsTestScheme registers corev1 alongside the operator's types; the
// shared helper in the service-connection tests covers only the latter.
func secretsTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("registering corev1: %v", err)
	}
	if err := authentikv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("registering authentik types: %v", err)
	}
	return scheme
}

func testProvider(uid types.UID) *authentikv1alpha1.OAuth2Provider {
	p := &authentikv1alpha1.OAuth2Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "grafana", Namespace: "apps", UID: uid},
	}
	p.SetGroupVersionKind(authentikv1alpha1.GroupVersion.WithKind("OAuth2Provider"))
	return p
}

func TestAssertSecretWritable(t *testing.T) {
	scheme := secretsTestScheme(t)
	owner := testProvider("owner-uid")
	key := types.NamespacedName{Name: "target", Namespace: "apps"}

	ownedByUs := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "target", Namespace: "apps",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: authentikv1alpha1.GroupVersion.String(),
				Kind:       "OAuth2Provider",
				Name:       "grafana",
				UID:        "owner-uid",
				Controller: ptr(true),
			}},
		},
	}
	unowned := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: "apps"},
		Data:       map[string][]byte{"password": []byte("postgres-password")},
	}
	ownedByAnother := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "target", Namespace: "apps",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: authentikv1alpha1.GroupVersion.String(),
				Kind:       "OAuth2Provider",
				Name:       "grafana",
				UID:        "a-different-uid",
				Controller: ptr(true),
			}},
		},
	}

	cases := []struct {
		name     string
		existing *corev1.Secret
		wantErr  bool
	}{
		{
			name:     "a Secret that does not exist yet is writable",
			existing: nil,
		},
		{
			name:     "a Secret this resource already controls is writable",
			existing: ownedByUs,
		},
		{
			// The exploit: pointing writeCredentialsTo at someone else's Secret.
			name:     "an unowned Secret is refused",
			existing: unowned,
			wantErr:  true,
		},
		{
			// A recreated owner of the same name is a different object and must
			// not inherit the previous one's Secret.
			name:     "a Secret owned by a different UID is refused",
			existing: ownedByAnother,
			wantErr:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tc.existing != nil {
				builder = builder.WithObjects(tc.existing)
			}
			c := builder.Build()

			err := assertSecretWritable(context.Background(), c, key, owner)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected the write to be refused")
				}
				if !IsSecretOwnershipError(err) {
					t.Fatalf("expected a SecretOwnershipError, got %v", err)
				}
				// It will never become ours, so retrying would only overwrite
				// the victim later.
				reason, requeue := ResultFor(err)
				if reason != authentikv1alpha1.ReasonSecretConflict {
					t.Errorf("reason = %q, want %q", reason, authentikv1alpha1.ReasonSecretConflict)
				}
				if requeue {
					t.Error("a Secret conflict must not requeue")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected the write to be allowed, got %v", err)
			}
		})
	}
}

// The message has to tell an operator what to do; "forbidden" alone sends them
// to the source.
func TestSecretOwnershipErrorIsActionable(t *testing.T) {
	err := &SecretOwnershipError{
		Secret: types.NamespacedName{Name: "postgres-creds", Namespace: "apps"},
		Owner:  "OAuth2Provider grafana",
	}
	msg := err.Error()
	for _, want := range []string{"postgres-creds", "OAuth2Provider grafana", "Choose a name"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message does not mention %q: %s", want, msg)
		}
	}
}

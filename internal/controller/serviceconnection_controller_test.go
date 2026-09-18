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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
)

// testScheme builds the scheme the fake client needs for these tests.
func serviceConnectionScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := authentikv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("adding the operator types to the scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("adding the core types to the scheme: %v", err)
	}
	return scheme
}

// sampleKubeconfig is a minimal but structurally real kubeconfig.
const sampleKubeconfig = `
apiVersion: v1
kind: Config
clusters:
  - name: prod
    cluster:
      server: https://prod.example:6443
current-context: prod
`

func TestDesiredKubeconfig(t *testing.T) {
	scheme := serviceConnectionScheme(t)

	newConn := func(ref *authentikv1alpha1.LocalSecretKeyReference) *authentikv1alpha1.KubernetesServiceConnection {
		conn := &authentikv1alpha1.KubernetesServiceConnection{
			Spec: authentikv1alpha1.KubernetesServiceConnectionSpec{KubeconfigSecretRef: ref},
		}
		conn.Name = "cluster"
		conn.Namespace = "team-a"
		return conn
	}

	cases := []struct {
		name    string
		ref     *authentikv1alpha1.LocalSecretKeyReference
		secret  *corev1.Secret
		wantNil bool
		wantErr bool
	}{
		{
			// A local connection has nothing to read, and must not be turned
			// into a credential lookup that can fail.
			name:    "no reference reads nothing",
			wantNil: true,
		},
		{
			name: "a valid kubeconfig is parsed",
			ref:  &authentikv1alpha1.LocalSecretKeyReference{Name: "kubeconfig", Key: "config"},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "kubeconfig", Namespace: "team-a"},
				Data:       map[string][]byte{"config": []byte(sampleKubeconfig)},
			},
		},
		{
			// The Secret is the operator's only source of cluster credentials,
			// so its absence has to be an error rather than a silent local
			// connection.
			name:    "a missing secret fails",
			ref:     &authentikv1alpha1.LocalSecretKeyReference{Name: "kubeconfig", Key: "config"},
			wantErr: true,
		},
		{
			name: "a missing key fails",
			ref:  &authentikv1alpha1.LocalSecretKeyReference{Name: "kubeconfig", Key: "config"},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "kubeconfig", Namespace: "team-a"},
				Data:       map[string][]byte{"other": []byte(sampleKubeconfig)},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tc.secret != nil {
				builder = builder.WithObjects(tc.secret)
			}
			r := &KubernetesServiceConnectionReconciler{Client: builder.Build(), Scheme: scheme}

			got, err := r.desiredKubeconfig(context.Background(), newConn(tc.ref))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				if !errors.Is(err, ErrCredentialsUnavailable) {
					t.Errorf("err = %v, want ErrCredentialsUnavailable so the resource requeues", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("desiredKubeconfig: %v", err)
			}
			if tc.wantNil {
				if got != nil {
					t.Errorf("got %v, want no kubeconfig", got)
				}
				return
			}
			if got["kind"] != "Config" {
				t.Errorf("parsed kubeconfig = %v, want the decoded document", got)
			}
		})
	}
}

// A kubeconfig holds client certificates and bearer tokens. The parser's own
// error quotes the line that failed to parse, so it must never reach a
// condition message or a log line.
func TestParseKubeconfigDoesNotLeakContents(t *testing.T) {
	secretName := "kubeconfig"
	broken := []byte("client-key-data: SUPERSECRETKEYMATERIAL\n\tbad indentation: [")

	_, err := parseKubeconfig(broken, types.NamespacedName{Namespace: "team-a", Name: secretName}, "config")
	if err == nil {
		t.Fatal("expected a parse failure")
	}
	if strings.Contains(err.Error(), "SUPERSECRETKEYMATERIAL") {
		t.Fatalf("the error leaked the kubeconfig contents: %v", err)
	}
	if !errors.Is(err, ErrCredentialsUnavailable) {
		// Reported as a missing credential, not a rejected spec: the fix is a
		// Secret edit, which produces no event here, so a resource that gave
		// up retrying would stay broken until the operator restarted.
		t.Errorf("err = %v, want ErrCredentialsUnavailable so the resource keeps retrying", err)
	}
}

// A document that parses as YAML but is not a mapping (a bare string, a list)
// is not a kubeconfig and must be rejected rather than sent on as an empty
// configuration that authentik would accept and then fail to use.
func TestParseKubeconfigRejectsNonMapping(t *testing.T) {
	for _, input := range []string{"just a string", "- a\n- list\n", ""} {
		if _, err := parseKubeconfig([]byte(input), types.NamespacedName{Namespace: "team-a", Name: "kubeconfig"}, "config"); err == nil {
			t.Errorf("input %q was accepted as a kubeconfig", input)
		}
	}
}

func TestRecordServiceConnectionIdentity(t *testing.T) {
	var status authentikv1alpha1.ServiceConnectionStatus

	recordServiceConnectionIdentity(&status, SyncOutcome{RemoteID: "b1d1", Adopted: true}, "cluster", "https://authentik.example")

	if status.RemoteID != "b1d1" || status.ServiceConnectionID != "b1d1" {
		t.Errorf("status = %+v, want the UUID recorded in both fields", status)
	}
	if !status.Adopted {
		t.Error("adoption must be recorded")
	}

	// Adoption is sticky: a later reconcile that merely updates the object
	// must not make it look like the operator created it after all.
	recordServiceConnectionIdentity(&status, SyncOutcome{RemoteID: "b1d1"}, "cluster", "https://authentik.example")
	if !status.Adopted {
		t.Error("a later reconcile must not clear the adoption record")
	}
}

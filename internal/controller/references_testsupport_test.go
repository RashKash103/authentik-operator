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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
)

// referenceFixture builds a Kubernetes client holding resolved Flow,
// PropertyMapping and CertificateKeyPair resources for the given names.
//
// Adapter tests exercise request building, not reference resolution, so they
// need every reference to resolve without each test assembling the same three
// kinds by hand. Anything not listed here is absent, which is how a test asserts
// the unresolved path.
func referenceFixture(t *testing.T, namespace string, names ...string) client.Client {
	t.Helper()

	scheme := serviceConnectionScheme(t)
	objects := make([]client.Object, 0, len(names)*3)

	// A name that is referenced resolves regardless of which kind the
	// reference expects; a test asserting the unresolved path simply uses a
	// name that is not listed.
	for _, name := range names {
		flow := &authentikv1alpha1.Flow{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       authentikv1alpha1.FlowSpec{ExistingSlug: name},
		}
		// A predictable ID so assertions can name the expected value.
		flow.Status.RemoteID = name + "-uuid"
		objects = append(objects, flow)

		mapping := &authentikv1alpha1.PropertyMapping{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       authentikv1alpha1.PropertyMappingSpec{ExistingName: name},
		}
		mapping.Status.RemoteID = name + "-uuid"
		objects = append(objects, mapping)

		pair := &authentikv1alpha1.CertificateKeyPair{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       authentikv1alpha1.CertificateKeyPairSpec{ExistingName: name},
		}
		pair.Status.RemoteID = name + "-uuid"
		objects = append(objects, pair)
	}

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

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
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// Reference resolution shared by every adapter.
//
// A reference names a Flow, PropertyMapping or CertificateKeyPair resource, and
// resolution reads that resource's status.remoteID. It deliberately does not
// look the object up in authentik by name: going through the Kubernetes object
// keeps it the source of truth, and means renaming something inside authentik
// cannot silently repoint a provider at a different object.
//
// It also means these resources gain the ability to *create* what they describe
// without anything here changing — the referrer only ever reads a UUID.

// referenceNotReady builds the error for a reference that cannot be resolved
// yet.
//
// It wraps ErrNotFound so ResultFor requeues rather than failing permanently: a
// provider applied before its Flow is normal, and must converge once the Flow
// reports an ID.
func referenceNotReady(field, kind, name, why string) error {
	return fmt.Errorf("%w: %s: %s %q %s", authentik.ErrNotFound, field, kind, name, why)
}

// resolveFlowRef resolves a required Flow reference to its authentik UUID.
func resolveFlowRef(
	ctx context.Context, c client.Client, namespace, field string,
	ref authentikv1alpha1.FlowReference,
) (string, error) {
	var flow authentikv1alpha1.Flow
	key := types.NamespacedName{Name: ref.Name, Namespace: namespace}
	if err := c.Get(ctx, key, &flow); err != nil {
		if apierrors.IsNotFound(err) {
			return "", referenceNotReady(field, "Flow", ref.Name, "does not exist")
		}
		return "", err
	}
	if flow.Status.RemoteID == "" {
		return "", referenceNotReady(field, "Flow", ref.Name, "has not resolved in authentik yet")
	}
	return flow.Status.RemoteID, nil
}

// resolveOptionalFlowRef resolves a Flow reference that may be unset.
func resolveOptionalFlowRef(
	ctx context.Context, c client.Client, namespace, field string,
	ref *authentikv1alpha1.FlowReference,
) (string, error) {
	if ref == nil || ref.Name == "" {
		return "", nil
	}
	return resolveFlowRef(ctx, c, namespace, field, *ref)
}

// resolveKeyPairRef resolves an optional CertificateKeyPair reference.
func resolveKeyPairRef(
	ctx context.Context, c client.Client, namespace, field string,
	ref *authentikv1alpha1.CertificateKeyPairReference,
) (string, error) {
	if ref == nil || ref.Name == "" {
		return "", nil
	}

	var pair authentikv1alpha1.CertificateKeyPair
	key := types.NamespacedName{Name: ref.Name, Namespace: namespace}
	if err := c.Get(ctx, key, &pair); err != nil {
		if apierrors.IsNotFound(err) {
			return "", referenceNotReady(field, "CertificateKeyPair", ref.Name, "does not exist")
		}
		return "", err
	}
	if pair.Status.RemoteID == "" {
		return "", referenceNotReady(field, "CertificateKeyPair", ref.Name,
			"has not resolved in authentik yet")
	}
	return pair.Status.RemoteID, nil
}

// resolvePropertyMappingRef resolves an optional PropertyMapping reference.
func resolvePropertyMappingRef(
	ctx context.Context, c client.Client, namespace, field string,
	ref *authentikv1alpha1.PropertyMappingReference,
) (string, error) {
	if ref == nil || ref.Name == "" {
		return "", nil
	}

	var mapping authentikv1alpha1.PropertyMapping
	key := types.NamespacedName{Name: ref.Name, Namespace: namespace}
	if err := c.Get(ctx, key, &mapping); err != nil {
		if apierrors.IsNotFound(err) {
			return "", referenceNotReady(field, "PropertyMapping", ref.Name, "does not exist")
		}
		return "", err
	}
	if mapping.Status.RemoteID == "" {
		return "", referenceNotReady(field, "PropertyMapping", ref.Name,
			"has not resolved in authentik yet")
	}
	return mapping.Status.RemoteID, nil
}

// resolvePropertyMappingRefs resolves a list of references, preserving order.
//
// It fails on the first unresolvable entry rather than sending a partial list:
// a provider attached to half its property mappings issues tokens missing
// claims, which fails somewhere far away from here.
func resolvePropertyMappingRefs(
	ctx context.Context, c client.Client, namespace, field string,
	refs []authentikv1alpha1.PropertyMappingReference,
) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}

	out := make([]string, 0, len(refs))
	for i := range refs {
		ref := refs[i]
		resolved, err := resolvePropertyMappingRef(ctx, c, namespace,
			fmt.Sprintf("%s[%d]", field, i), &ref)
		if err != nil {
			return nil, err
		}
		if resolved != "" {
			out = append(out, resolved)
		}
	}
	return out, nil
}

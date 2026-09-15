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

// ReferencePolicy decides whether a reference may cross namespaces.
//
// Off by default. Sharing a Flow between namespaces is a legitimate thing to
// want, but it also means one team's configuration silently becomes another's
// dependency, so it is an operator-level decision rather than something any
// manifest author can turn on for themselves.
type ReferencePolicy struct {
	// AllowCrossNamespace permits a reference to name another namespace.
	AllowCrossNamespace bool
}

// ErrCrossNamespaceDisabled reports a reference blocked by policy.
var ErrCrossNamespaceDisabled = errors.New("cross-namespace references are disabled")

// resolveNamespace decides which namespace a reference resolves in, and
// refuses one that crosses namespaces when policy forbids it.
//
// A blocked reference is an error rather than a silent fallback to the local
// namespace: falling back would resolve a different object than the manifest
// names, which is worse than refusing.
func resolveNamespace(policy ReferencePolicy, field, own, requested string) (string, error) {
	if requested == "" || requested == own {
		return own, nil
	}
	if !policy.AllowCrossNamespace {
		return "", fmt.Errorf(
			"%w: %s names namespace %q, but the operator was started without "+
				"--allow-cross-namespace-references",
			ErrCrossNamespaceDisabled, field, requested)
	}
	return requested, nil
}

// assertSameInstance refuses a reference resolved against a different authentik.
//
// A UUID only means anything on the instance that issued it. Without this,
// referencing a Flow from a namespace wired to a different authentik would send
// that instance a UUID it has never seen - and authentik's error for that is
// not one anybody could act on.
func assertSameInstance(field, kind, name, referenced, own string) error {
	if referenced == "" || own == "" || referenced == own {
		return nil
	}
	return fmt.Errorf(
		"%w: %s: %s %q resolved against %s, but this resource uses %s; "+
			"a reference must point at the same authentik instance",
		authentik.ErrValidation, field, kind, name, referenced, own)
}

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
	ctx context.Context, c client.Client, scope ReferenceScope, field string,
	ref authentikv1alpha1.FlowReference,
) (string, error) {
	ns, err := resolveNamespace(scope.Policy, field, scope.Namespace, ref.Namespace)
	if err != nil {
		return "", err
	}

	var flow authentikv1alpha1.Flow
	key := types.NamespacedName{Name: ref.Name, Namespace: ns}
	if err := c.Get(ctx, key, &flow); err != nil {
		if apierrors.IsNotFound(err) {
			return "", referenceNotReady(field, "Flow", ref.Name, "does not exist")
		}
		return "", err
	}
	if flow.Status.RemoteID == "" {
		return "", referenceNotReady(field, "Flow", ref.Name, "has not resolved in authentik yet")
	}
	if err := assertSameInstance(field, "Flow", ref.Name, flow.Status.AuthentikURL, scope.AuthentikURL); err != nil {
		return "", err
	}
	return flow.Status.RemoteID, nil
}

// resolveOptionalFlowRef resolves a Flow reference that may be unset.
func resolveOptionalFlowRef(
	ctx context.Context, c client.Client, scope ReferenceScope, field string,
	ref *authentikv1alpha1.FlowReference,
) (string, error) {
	if ref == nil || ref.Name == "" {
		return "", nil
	}
	return resolveFlowRef(ctx, c, scope, field, *ref)
}

// resolveKeyPairRef resolves an optional CertificateKeyPair reference.
func resolveKeyPairRef(
	ctx context.Context, c client.Client, scope ReferenceScope, field string,
	ref *authentikv1alpha1.CertificateKeyPairReference,
) (string, error) {
	if ref == nil || ref.Name == "" {
		return "", nil
	}

	ns, err := resolveNamespace(scope.Policy, field, scope.Namespace, ref.Namespace)
	if err != nil {
		return "", err
	}

	var pair authentikv1alpha1.CertificateKeyPair
	key := types.NamespacedName{Name: ref.Name, Namespace: ns}
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
	if err := assertSameInstance(field, "CertificateKeyPair", ref.Name,
		pair.Status.AuthentikURL, scope.AuthentikURL); err != nil {
		return "", err
	}
	return pair.Status.RemoteID, nil
}

// resolvePropertyMappingRef resolves an optional PropertyMapping reference.
func resolvePropertyMappingRef(
	ctx context.Context, c client.Client, scope ReferenceScope, field string,
	ref *authentikv1alpha1.PropertyMappingReference,
) (string, error) {
	if ref == nil || ref.Name == "" {
		return "", nil
	}

	ns, err := resolveNamespace(scope.Policy, field, scope.Namespace, ref.Namespace)
	if err != nil {
		return "", err
	}

	var mapping authentikv1alpha1.PropertyMapping
	key := types.NamespacedName{Name: ref.Name, Namespace: ns}
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
	if err := assertSameInstance(field, "PropertyMapping", ref.Name,
		mapping.Status.AuthentikURL, scope.AuthentikURL); err != nil {
		return "", err
	}
	return mapping.Status.RemoteID, nil
}

// resolvePropertyMappingRefs resolves a list of references, preserving order.
//
// It fails on the first unresolvable entry rather than sending a partial list:
// a provider attached to half its property mappings issues tokens missing
// claims, which fails somewhere far away from here.
func resolvePropertyMappingRefs(
	ctx context.Context, c client.Client, scope ReferenceScope, field string,
	refs []authentikv1alpha1.PropertyMappingReference,
) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}

	out := make([]string, 0, len(refs))
	for i := range refs {
		ref := refs[i]
		resolved, err := resolvePropertyMappingRef(ctx, c, scope,
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

// ReferenceScope is the context a reference resolves in: whose namespace it
// defaults to, which authentik the referrer talks to, and whether crossing
// namespaces is permitted.
type ReferenceScope struct {
	// Namespace of the referring resource.
	Namespace string
	// AuthentikURL the referring resource's connection points at. Empty skips
	// the same-instance check, which is what unit tests want.
	AuthentikURL string
	// Policy decides whether a reference may name another namespace.
	Policy ReferencePolicy
}

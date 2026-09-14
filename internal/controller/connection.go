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
	"slices"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// Errors returned when a connection cannot be turned into a usable client.
// Controllers branch on these rather than matching strings.
var (
	// ErrConnectionNotFound means the referenced connection object is absent.
	ErrConnectionNotFound = errors.New("connection not found")
	// ErrConnectionNotReady means the connection exists but is not usable yet.
	ErrConnectionNotReady = errors.New("connection not ready")
	// ErrNamespaceNotAllowed means a cluster connection refuses this namespace.
	ErrNamespaceNotAllowed = errors.New("namespace not allowed to use this connection")
	// ErrCredentialsUnavailable means the token Secret or key is missing.
	ErrCredentialsUnavailable = errors.New("connection credentials unavailable")
)

// ConnectionResolver turns a ConnectionReference into an authentik client.
//
// One resolver is shared by every controller so that resolved references are
// cached once per authentik instance rather than once per controller.
type ConnectionResolver struct {
	Client client.Client
	Cache  *authentik.RefCache

	// UserAgentSuffix is appended to the User-Agent of every client built here.
	UserAgentSuffix string
}

// Resolve builds an authentik client for the given reference.
//
// A namespaced AuthentikConnection is always read from callerNamespace. There
// is deliberately no way to reach one in another namespace: that would let
// anyone able to create a resource in one namespace borrow credentials from
// another, which is the classic privilege-escalation bug in operators.
func (r *ConnectionResolver) Resolve(ctx context.Context, ref authentikv1alpha1.ConnectionReference, callerNamespace string) (authentik.Client, error) {
	kind := ref.Kind
	if kind == "" {
		kind = authentikv1alpha1.ConnectionKindNamespaced
	}

	switch kind {
	case authentikv1alpha1.ConnectionKindNamespaced:
		return r.resolveNamespaced(ctx, ref.Name, callerNamespace)
	case authentikv1alpha1.ConnectionKindCluster:
		return r.resolveCluster(ctx, ref.Name, callerNamespace)
	default:
		return nil, fmt.Errorf("unknown connection kind %q", kind)
	}
}

func (r *ConnectionResolver) resolveNamespaced(ctx context.Context, name, namespace string) (authentik.Client, error) {
	var conn authentikv1alpha1.AuthentikConnection
	key := types.NamespacedName{Name: name, Namespace: namespace}
	if err := r.Client.Get(ctx, key, &conn); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: AuthentikConnection %s", ErrConnectionNotFound, key)
		}
		return nil, err
	}

	if !meta.IsStatusConditionTrue(conn.Status.Conditions, authentikv1alpha1.ConditionReady) {
		return nil, fmt.Errorf("%w: AuthentikConnection %s", ErrConnectionNotReady, key)
	}

	// The Secret is read from the connection's own namespace, which is also the
	// caller's namespace by construction.
	tokenRef := authentikv1alpha1.SecretKeyReference{
		Name: conn.Spec.TokenSecretRef.Name, Namespace: namespace, Key: conn.Spec.TokenSecretRef.Key,
	}
	var caRef *authentikv1alpha1.SecretKeyReference
	if conn.Spec.CABundleSecretRef != nil {
		caRef = &authentikv1alpha1.SecretKeyReference{
			Name: conn.Spec.CABundleSecretRef.Name, Namespace: namespace, Key: conn.Spec.CABundleSecretRef.Key,
		}
	}
	return r.build(ctx, conn.Spec.ConnectionSettings, tokenRef, caRef)
}

func (r *ConnectionResolver) resolveCluster(ctx context.Context, name, callerNamespace string) (authentik.Client, error) {
	var conn authentikv1alpha1.ClusterAuthentikConnection
	key := types.NamespacedName{Name: name}
	if err := r.Client.Get(ctx, key, &conn); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: ClusterAuthentikConnection %s", ErrConnectionNotFound, name)
		}
		return nil, err
	}

	// An empty allowlist means every namespace may use the connection; a
	// populated one is checked before any credential is read, so a namespace
	// that is not permitted never causes a Secret lookup.
	if len(conn.Spec.AllowedNamespaces) > 0 && !slices.Contains(conn.Spec.AllowedNamespaces, callerNamespace) {
		return nil, fmt.Errorf("%w: namespace %q is not in the allowedNamespaces of ClusterAuthentikConnection %s",
			ErrNamespaceNotAllowed, callerNamespace, name)
	}

	if !meta.IsStatusConditionTrue(conn.Status.Conditions, authentikv1alpha1.ConditionReady) {
		return nil, fmt.Errorf("%w: ClusterAuthentikConnection %s", ErrConnectionNotReady, name)
	}

	return r.build(ctx, conn.Spec.ConnectionSettings, conn.Spec.TokenSecretRef, conn.Spec.CABundleSecretRef)
}

// build reads the credentials and constructs the client.
func (r *ConnectionResolver) build(
	ctx context.Context,
	settings authentikv1alpha1.ConnectionSettings,
	tokenRef authentikv1alpha1.SecretKeyReference,
	caRef *authentikv1alpha1.SecretKeyReference,
) (authentik.Client, error) {
	token, err := r.readSecretKey(ctx, tokenRef)
	if err != nil {
		return nil, err
	}

	var caBundle []byte
	if caRef != nil {
		caBundle, err = r.readSecretKey(ctx, *caRef)
		if err != nil {
			return nil, err
		}
	}

	return authentik.New(authentik.Config{
		BaseURL:            settings.URL,
		Token:              string(token),
		CABundle:           caBundle,
		InsecureSkipVerify: settings.InsecureSkipTLSVerify,
		Cache:              r.Cache,
		UserAgentSuffix:    r.UserAgentSuffix,
	})
}

// readSecretKey fetches one key of a Secret.
//
// The returned error never contains the secret value, and the key's absence is
// reported as a distinct error so the caller can surface an actionable message
// rather than a generic failure.
func (r *ConnectionResolver) readSecretKey(ctx context.Context, ref authentikv1alpha1.SecretKeyReference) ([]byte, error) {
	var secret corev1.Secret
	key := types.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}
	if err := r.Client.Get(ctx, key, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: Secret %s not found", ErrCredentialsUnavailable, key)
		}
		return nil, err
	}

	value, ok := secret.Data[ref.Key]
	if !ok || len(value) == 0 {
		return nil, fmt.Errorf("%w: Secret %s has no non-empty key %q", ErrCredentialsUnavailable, key, ref.Key)
	}
	return value, nil
}

// ConnectionProblemReason maps a resolution failure to the condition reason a
// dependent resource should report.
func ConnectionProblemReason(err error) string {
	switch {
	case errors.Is(err, ErrConnectionNotFound), errors.Is(err, ErrCredentialsUnavailable):
		return authentikv1alpha1.ReasonReferenceNotFound
	case errors.Is(err, ErrConnectionNotReady), errors.Is(err, ErrNamespaceNotAllowed):
		return authentikv1alpha1.ReasonConnectionNotReady
	default:
		return authentikv1alpha1.ReasonAPIError
	}
}

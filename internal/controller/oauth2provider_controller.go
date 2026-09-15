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
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/recorder"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// retryAfterFailure bounds how fast a failing resource is retried.
const retryAfterFailure = 30 * time.Second

// OAuth2ProviderReconciler reconciles an OAuth2Provider.
type OAuth2ProviderReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder recorder.EventRecorder
	Resolver *ConnectionResolver
}

// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=oauth2providers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=oauth2providers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=oauth2providers/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile brings one OAuth2Provider in line with authentik.
func (r *OAuth2ProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var provider authentikv1alpha1.OAuth2Provider
	if err := r.Get(ctx, req.NamespacedName, &provider); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	akClient, connErr := r.Resolver.Resolve(ctx, provider.Spec.ConnectionRef, provider.Namespace)

	if !provider.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &provider, akClient, connErr)
	}

	if connErr != nil {
		return r.fail(ctx, &provider, connErr)
	}

	if controllerutil.AddFinalizer(&provider, Finalizer) {
		// Persist the finalizer before creating anything remotely. The other
		// order can leak an authentik object: if the process dies after the
		// create, nothing records that cleanup is owed.
		if err := r.Update(ctx, &provider); err != nil {
			return ctrl.Result{}, err
		}
	}

	clientSecret, err := r.desiredClientSecret(ctx, &provider)
	if err != nil {
		return r.fail(ctx, &provider, err)
	}

	rotating := rotationRequested(&provider)
	if rotating {
		if provider.Spec.ClientSecretRef != nil {
			// The secret is caller-supplied, so rotating it here would
			// immediately be overwritten by the referenced Secret. Rotate that
			// Secret instead.
			return r.fail(ctx, &provider, errRotationWithFixedSecret)
		}
		clientSecret, err = generateClientSecret()
		if err != nil {
			return r.fail(ctx, &provider, err)
		}
	}

	adapter := &oauth2Adapter{client: akClient, kube: r.Client, provider: &provider, clientSecret: clientSecret}
	outcome, err := Sync(ctx, SyncRequest{
		Object: &provider, Adapter: adapter, Recorder: r.Recorder, Scheme: r.Scheme,
	})
	if err != nil {
		return r.fail(ctx, &provider, err)
	}

	// Record identity before publishing credentials: writeCredentials needs
	// the provider's primary key to look up its discovery URLs.
	r.recordIdentity(&provider, outcome, adapter)

	if err := r.writeCredentials(ctx, &provider, adapter); err != nil {
		return r.fail(ctx, &provider, err)
	}

	if rotating {
		now := metav1.Now()
		provider.Status.CredentialsRotatedAt = &now
		provider.Status.ObservedRotationToken = provider.Annotations[authentikv1alpha1.RotateCredentialsAnnotation]
		r.recordRotation(&provider)
	}

	MarkReady(&provider.Status.Conditions, provider.Generation)
	if err := r.Status().Update(ctx, &provider); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.V(1).Info("reconciled oauth2 provider",
		"provider", provider.ProviderName(), "remoteID", outcome.RemoteID,
		"created", outcome.Created, "adopted", outcome.Adopted, "updated", outcome.Updated)
	return ctrl.Result{}, nil
}

// reconcileDelete tears down the authentik provider and releases the finalizer.
func (r *OAuth2ProviderReconciler) reconcileDelete(
	ctx context.Context,
	provider *authentikv1alpha1.OAuth2Provider,
	akClient authentik.Client,
	connErr error,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(provider, Finalizer) {
		return ctrl.Result{}, nil
	}

	// Without a usable connection the remote object cannot be removed. Keep
	// retrying rather than dropping the finalizer, which would silently leave
	// a provider behind in authentik.
	if connErr != nil {
		MarkNotReady(&provider.Status.Conditions, ConnectionProblemReason(connErr),
			"cannot delete the authentik provider: "+connErr.Error(), provider.Generation)
		if err := r.Status().Update(ctx, provider); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		return ctrl.Result{RequeueAfter: retryAfterFailure}, nil
	}

	adapter := &oauth2Adapter{client: akClient, kube: r.Client, provider: provider}
	done, err := Finalize(ctx, SyncRequest{Object: provider, Adapter: adapter, Recorder: r.Recorder})
	if err != nil {
		return ctrl.Result{RequeueAfter: retryAfterFailure}, nil //nolint:nilerr // reported via condition
	}
	if !done {
		return ctrl.Result{RequeueAfter: retryAfterFailure}, nil
	}

	controllerutil.RemoveFinalizer(provider, Finalizer)
	return ctrl.Result{}, client.IgnoreNotFound(r.Update(ctx, provider))
}

// desiredClientSecret reads a caller-supplied client secret, if there is one.
func (r *OAuth2ProviderReconciler) desiredClientSecret(
	ctx context.Context, provider *authentikv1alpha1.OAuth2Provider,
) (string, error) {
	ref := provider.Spec.ClientSecretRef
	if ref == nil {
		// Left to authentik, which generates one.
		return "", nil
	}

	var secret corev1.Secret
	key := types.NamespacedName{Name: ref.Name, Namespace: provider.Namespace}
	if err := r.Get(ctx, key, &secret); err != nil {
		return "", fmt.Errorf("%w: reading clientSecretRef: %s", ErrCredentialsUnavailable, key)
	}
	value, ok := secret.Data[ref.Key]
	if !ok || len(value) == 0 {
		return "", fmt.Errorf("%w: Secret %s has no non-empty key %q", ErrCredentialsUnavailable, key, ref.Key)
	}
	return string(value), nil
}

// writeCredentials publishes the client id and secret into a Secret so the
// workload that needs them can mount it.
//
// The values are never placed in status or in a log line: status is readable
// by anyone who can read the custom resource, which is a wider audience than
// those who can read Secrets in the namespace.
func (r *OAuth2ProviderReconciler) writeCredentials(
	ctx context.Context, provider *authentikv1alpha1.OAuth2Provider, adapter *oauth2Adapter,
) error {
	target := provider.Spec.WriteCredentialsTo
	if target == nil || adapter.observed == nil {
		return nil
	}

	clientID := adapter.observed.GetClientId()
	clientSecret := adapter.observed.GetClientSecret()
	if clientID == "" {
		// Nothing useful to publish yet.
		return nil
	}

	// Best effort: the credentials are the point of the Secret, so a failure
	// to read the discovery URLs must not stop them being published.
	urls := adapter.setupURLs(ctx, provider.Status.ProviderID)

	key := types.NamespacedName{Name: target.Name, Namespace: provider.Namespace}

	// The target name comes from the spec, so it must be checked before it is
	// written to. See assertSecretWritable.
	if err := assertSecretWritable(ctx, r.Client, key, provider); err != nil {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
	}

	// Create-or-update rather than delete-and-recreate: recreating the Secret
	// would break every Pod that already has it mounted.
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.Labels == nil {
			secret.Labels = map[string]string{}
		}
		secret.Labels["app.kubernetes.io/managed-by"] = "authentik-operator"
		secret.Labels["authentik.k8s.rka.sh/provider"] = provider.Name
		secret.Type = corev1.SecretTypeOpaque

		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		secret.Data[keyOrDefault(target.ClientIDKey, "client-id")] = []byte(clientID)
		if clientSecret != "" {
			secret.Data[keyOrDefault(target.ClientSecretKey, "client-secret")] = []byte(clientSecret)
		}
		if urls != nil {
			// An OIDC client needs the issuer to discover everything else, so
			// publishing it alongside the credentials saves the consumer from
			// hardcoding a URL that changes with issuerMode.
			secret.Data[keyOrDefault(target.IssuerKey, "issuer")] = []byte(urls.Issuer)
		}

		// Owned by the provider, so it is garbage-collected with it.
		return controllerutil.SetControllerReference(provider, secret, r.Scheme)
	})
	if err != nil {
		return fmt.Errorf("writing credentials secret: %w", err)
	}

	provider.Status.CredentialsSecretName = target.Name
	return nil
}

// recordIdentity writes the authentik identity of the provider onto status.
func (r *OAuth2ProviderReconciler) recordIdentity(
	provider *authentikv1alpha1.OAuth2Provider, outcome SyncOutcome, adapter *oauth2Adapter,
) {
	status := provider.ManagedStatus()
	status.RemoteID = outcome.RemoteID
	status.RemoteName = provider.ProviderName()
	status.Adopted = status.Adopted || outcome.Adopted
	now := metav1.Now()
	status.LastSyncedTime = &now

	if pk, err := strconv.ParseInt(outcome.RemoteID, 10, 32); err == nil {
		id := int32(pk)
		provider.Status.ProviderID = &id
	}
	if adapter.observed != nil {
		provider.Status.ClientID = adapter.observed.GetClientId()
	}
}

// fail records a failed reconcile and decides whether to retry.
func (r *OAuth2ProviderReconciler) fail(
	ctx context.Context, provider *authentikv1alpha1.OAuth2Provider, err error,
) (ctrl.Result, error) {
	reason, retry := ResultFor(err)

	if authentik.IsTransient(err) || reason == authentikv1alpha1.ReasonAPIError {
		MarkSyncFailed(&provider.Status.Conditions, reason, err.Error(), provider.Generation)
	} else {
		MarkNotReady(&provider.Status.Conditions, reason, err.Error(), provider.Generation)
	}
	provider.Status.ObservedGeneration = provider.Generation

	if updateErr := r.Status().Update(ctx, provider); updateErr != nil {
		return ctrl.Result{}, client.IgnoreNotFound(updateErr)
	}

	if !retry {
		// A spec authentik rejected, or an ambiguous reference, will not fix
		// itself. Waiting for the next spec change beats spinning.
		log.FromContext(ctx).Info("not retrying until the spec changes", "reason", reason, "error", err.Error())
		return ctrl.Result{}, nil
	}
	return ctrl.Result{RequeueAfter: retryAfterFailure}, nil
}

// SetupWithManager registers the controller.
func (r *OAuth2ProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&authentikv1alpha1.OAuth2Provider{}).
		Owns(&corev1.Secret{}).
		Named("oauth2provider").
		Complete(r)
}

// keyOrDefault returns value, or fallback when value is empty.
func keyOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// errRotationWithFixedSecret reports a rotation request the operator cannot
// satisfy.
var errRotationWithFixedSecret = errors.New(
	"cannot rotate a client secret supplied through spec.clientSecretRef; " +
		"rotate the referenced Secret instead, or remove clientSecretRef to let authentik generate one")

// rotationRequested reports whether the rotation annotation carries a value
// that has not been acted on yet.
//
// Comparing against the recorded token rather than simply reacting to the
// annotation's presence is what makes this idempotent: without it, every
// reconcile would mint a new secret and break running workloads.
func rotationRequested(provider *authentikv1alpha1.OAuth2Provider) bool {
	token, ok := provider.Annotations[authentikv1alpha1.RotateCredentialsAnnotation]
	if !ok || token == "" {
		return false
	}
	return token != provider.Status.ObservedRotationToken
}

// generateClientSecret mints a new client secret.
//
// The value is generated here rather than left to authentik because the update
// request has to carry it, and a caller-visible source of randomness keeps the
// rotation testable.
func generateClientSecret() (string, error) {
	buf := make([]byte, 48)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating client secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// recordRotation emits an Event so a rotation is visible in kubectl describe.
// The new value is never included.
func (r *OAuth2ProviderReconciler) recordRotation(provider *authentikv1alpha1.OAuth2Provider) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(provider, nil, corev1.EventTypeNormal,
		"CredentialsRotated", "CredentialsRotated",
		"Rotated the client secret; consuming workloads must reload to pick it up")
}

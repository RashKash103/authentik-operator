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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/recorder"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// outpostProviderRefIndexKey indexes outposts by the providers they reference,
// so that a provider finishing its own registration wakes the outposts waiting
// on it instead of leaving them to the requeue timer.
const outpostProviderRefIndexKey = ".spec.providerRefs"

// OutpostReconciler reconciles an Outpost.
type OutpostReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder recorder.EventRecorder
	Resolver *ConnectionResolver
	// References decides whether references may cross namespaces.
	References ReferencePolicy
}

// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=outposts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=outposts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=outposts/finalizers,verbs=update
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=oauth2providers,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile brings one Outpost in line with authentik.
func (r *OutpostReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var outpost authentikv1alpha1.Outpost
	if err := r.Get(ctx, req.NamespacedName, &outpost); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	akClient, connErr := r.Resolver.Resolve(ctx, outpost.Spec.ConnectionRef, outpost.Namespace)

	if !outpost.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &outpost, akClient, connErr)
	}

	if connErr != nil {
		return r.fail(ctx, &outpost, connErr)
	}

	if controllerutil.AddFinalizer(&outpost, Finalizer) {
		// Persist the finalizer before creating anything remotely. The other
		// order can leak an authentik object: if the process dies after the
		// create, nothing records that cleanup is owed.
		if err := r.Update(ctx, &outpost); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Every reference is resolved before anything is sent to authentik, so a
	// half-resolved outpost is never registered. See resolveProviderIDs.
	providerIDs, err := r.resolveProviderIDs(ctx, &outpost, akClient)
	if err != nil {
		return r.fail(ctx, &outpost, err)
	}

	serviceConnection, err := r.resolveServiceConnection(ctx, &outpost)
	if err != nil {
		return r.fail(ctx, &outpost, err)
	}

	adapter := &outpostAdapter{
		client:            akClient,
		outpost:           &outpost,
		providerIDs:       providerIDs,
		serviceConnection: serviceConnection,
	}
	outcome, err := Sync(ctx, SyncRequest{
		Object: &outpost, Adapter: adapter, Recorder: r.Recorder, Scheme: r.Scheme,
	})
	if err != nil {
		return r.fail(ctx, &outpost, err)
	}

	r.recordIdentity(&outpost, outcome, adapter, providerIDs, serviceConnection)

	if err := r.writeToken(ctx, &outpost, adapter); err != nil {
		return r.fail(ctx, &outpost, err)
	}

	MarkReady(&outpost.Status.Conditions, outpost.Generation)
	if err := r.Status().Update(ctx, &outpost); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.V(1).Info("reconciled outpost",
		"outpost", outpost.OutpostName(), "remoteID", outcome.RemoteID,
		"providers", len(providerIDs),
		"created", outcome.Created, "adopted", outcome.Adopted, "updated", outcome.Updated)
	return ctrl.Result{}, nil
}

// reconcileDelete tears down the authentik outpost and releases the finalizer.
func (r *OutpostReconciler) reconcileDelete(
	ctx context.Context,
	outpost *authentikv1alpha1.Outpost,
	akClient authentik.Client,
	connErr error,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(outpost, Finalizer) {
		return ctrl.Result{}, nil
	}

	// Without a usable connection the remote object cannot be removed. Keep
	// retrying rather than dropping the finalizer, which would silently leave
	// an outpost behind in authentik.
	if connErr != nil {
		MarkNotReady(&outpost.Status.Conditions, ConnectionProblemReason(connErr),
			"cannot delete the authentik outpost: "+connErr.Error(), outpost.Generation)
		if err := r.Status().Update(ctx, outpost); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		return ctrl.Result{RequeueAfter: retryAfterFailure}, nil
	}

	adapter := &outpostAdapter{client: akClient, outpost: outpost}
	done, err := Finalize(ctx, SyncRequest{Object: outpost, Adapter: adapter, Recorder: r.Recorder})
	if err != nil {
		return ctrl.Result{RequeueAfter: retryAfterFailure}, nil //nolint:nilerr // reported via condition
	}
	if !done {
		return ctrl.Result{RequeueAfter: retryAfterFailure}, nil
	}

	controllerutil.RemoveFinalizer(outpost, Finalizer)
	return ctrl.Result{}, client.IgnoreNotFound(r.Update(ctx, outpost))
}

// resolveProviderIDs turns every providerRef into the authentik primary key of
// the provider it names.
//
// Every reference resolves before anything reaches authentik, so a
// half-resolved outpost is never registered: an outpost serving a subset of its
// providers comes up and silently leaves the rest unprotected, which is worse
// than not coming up at all.
//
// Resolution is shared with the other consumers of ProviderReference, so an
// outpost can serve a provider that already exists in authentik and was not
// created here.
func (r *OutpostReconciler) resolveProviderIDs(
	ctx context.Context, outpost *authentikv1alpha1.Outpost, ak authentik.Client,
) ([]int32, error) {
	scope := ReferenceScope{
		Namespace:    outpost.Namespace,
		AuthentikURL: ak.BaseURL(),
		Policy:       r.References,
	}

	ids := make([]int32, 0, len(outpost.Spec.ProviderRefs))
	for i, ref := range outpost.Spec.ProviderRefs {
		field := fmt.Sprintf("spec.providerRefs[%d]", i)
		id, err := resolveProviderReference(ctx, r.Client, ak, scope, field, ref)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// resolveServiceConnection resolves the service connection reference to its
// UUID.
//
// Resolution goes through the referenced resource's status, so Kubernetes stays
// the source of truth. A service connection that authentik already has is
// adopted by declaring a KubernetesServiceConnection or DockerServiceConnection
// for it, rather than naming it inline here.
func (r *OutpostReconciler) resolveServiceConnection(
	ctx context.Context, outpost *authentikv1alpha1.Outpost,
) (string, error) {
	ref := outpost.Spec.ServiceConnectionRef
	if ref == nil {
		// No service connection: authentik registers the outpost but does not
		// deploy it, which is what a self-hosted outpost wants.
		return "", nil
	}

	switch {
	case ref.KubernetesServiceConnectionName != "":
		var sc authentikv1alpha1.KubernetesServiceConnection
		key := types.NamespacedName{Name: ref.KubernetesServiceConnectionName, Namespace: outpost.Namespace}
		if err := r.Get(ctx, key, &sc); err != nil {
			if apierrors.IsNotFound(err) {
				return "", serviceConnectionNotReady(key.Name, "KubernetesServiceConnection", "does not exist")
			}
			return "", err
		}
		if sc.Status.ServiceConnectionID == "" {
			return "", serviceConnectionNotReady(key.Name, "KubernetesServiceConnection",
				"has not been created in authentik yet")
		}
		return sc.Status.ServiceConnectionID, nil

	case ref.DockerServiceConnectionName != "":
		var sc authentikv1alpha1.DockerServiceConnection
		key := types.NamespacedName{Name: ref.DockerServiceConnectionName, Namespace: outpost.Namespace}
		if err := r.Get(ctx, key, &sc); err != nil {
			if apierrors.IsNotFound(err) {
				return "", serviceConnectionNotReady(key.Name, "DockerServiceConnection", "does not exist")
			}
			return "", err
		}
		if sc.Status.ServiceConnectionID == "" {
			return "", serviceConnectionNotReady(key.Name, "DockerServiceConnection",
				"has not been created in authentik yet")
		}
		return sc.Status.ServiceConnectionID, nil

	default:
		return "", fmt.Errorf("%w: spec.serviceConnectionRef names nothing", authentik.ErrValidation)
	}
}

// serviceConnectionNotReady builds the error for a service connection that
// cannot be resolved yet.
//
// It wraps ErrNotFound so ResultFor requeues: an outpost applied before its
// service connection is normal and must converge, not fail permanently.
func serviceConnectionNotReady(name, kind, why string) error {
	return fmt.Errorf("%w: spec.serviceConnectionRef: %s %q %s",
		authentik.ErrNotFound, kind, name, why)
}

// writeToken publishes the outpost's API token and the authentik base URL into
// a Secret, so a self-hosted outpost can be pointed at it.
//
// The token is never placed in status or in a log line: status is readable by
// anyone who can read the custom resource, which is a wider audience than
// those who can read Secrets in the namespace.
func (r *OutpostReconciler) writeToken(
	ctx context.Context, outpost *authentikv1alpha1.Outpost, adapter *outpostAdapter,
) error {
	target := outpost.Spec.WriteTokenTo
	if target == nil {
		return nil
	}

	token, err := adapter.token(ctx)
	if err != nil {
		return err
	}
	if token == "" {
		// Nothing useful to publish yet.
		return nil
	}

	secretKey := types.NamespacedName{Name: target.Name, Namespace: outpost.Namespace}

	// The target name comes from the spec, so it must be checked before it is
	// written to. See assertSecretWritable.
	if err := assertSecretWritable(ctx, r.Client, secretKey, outpost); err != nil {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretKey.Name, Namespace: secretKey.Namespace},
	}

	// Create-or-update rather than delete-and-recreate: recreating the Secret
	// would break every Pod that already has it mounted.
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.Labels == nil {
			secret.Labels = map[string]string{}
		}
		secret.Labels["app.kubernetes.io/managed-by"] = "authentik-operator"
		secret.Labels["authentik.k8s.rka.sh/outpost"] = outpost.Name
		secret.Type = corev1.SecretTypeOpaque

		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		secret.Data[keyOrDefault(target.TokenKey, "token")] = []byte(token)
		// An outpost needs somewhere to send the token as well as the token
		// itself, so the base URL travels with it rather than being configured
		// a second time by hand.
		secret.Data[keyOrDefault(target.HostKey, "authentik-host")] = []byte(adapter.client.BaseURL())

		// Owned by the outpost, so it is garbage-collected with it.
		return controllerutil.SetControllerReference(outpost, secret, r.Scheme)
	})
	if err != nil {
		return fmt.Errorf("writing outpost token secret: %w", err)
	}

	outpost.Status.TokenSecretName = target.Name
	return nil
}

// recordIdentity writes the authentik identity of the outpost onto status.
func (r *OutpostReconciler) recordIdentity(
	outpost *authentikv1alpha1.Outpost,
	outcome SyncOutcome,
	adapter *outpostAdapter,
	providerIDs []int32,
	serviceConnection string,
) {
	status := outpost.ManagedStatus()
	status.RemoteID = outcome.RemoteID
	// The scoped name, not the declared one: with a cluster identity set they
	// differ, and status has to report what authentik actually holds.
	status.RemoteName = adapter.DesiredName()
	status.AuthentikURL = adapter.client.BaseURL()
	status.Adopted = status.Adopted || outcome.Adopted
	now := metav1.Now()
	status.LastSyncedTime = &now

	outpost.Status.OutpostID = outcome.RemoteID
	outpost.Status.ProviderIDs = providerIDs
	outpost.Status.ServiceConnectionID = serviceConnection
	if adapter.observed != nil {
		outpost.Status.TokenIdentifier = adapter.observed.TokenIdentifier
	}
}

// fail records a failed reconcile and decides whether to retry.
func (r *OutpostReconciler) fail(
	ctx context.Context, outpost *authentikv1alpha1.Outpost, err error,
) (ctrl.Result, error) {
	reason, retry := ResultFor(err)
	markReconcileFailure(&outpost.Status.Conditions, reason, err, outpost.Generation)
	outpost.Status.ObservedGeneration = outpost.Generation

	if updateErr := r.Status().Update(ctx, outpost); updateErr != nil {
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

// SetupWithManager registers the controller, the provider index and the
// watches that feed it.
func (r *OutpostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(), &authentikv1alpha1.Outpost{}, outpostProviderRefIndexKey, indexOutpostProviderRefs,
	); err != nil {
		return err
	}

	// Watching every provider kind is what turns "applied out of order" into a
	// non-event: an outpost whose providers are not registered yet would
	// otherwise wait out the requeue timer, which is the difference between an
	// application coming back in seconds and coming back in half a minute.
	return ctrl.NewControllerManagedBy(mgr).
		For(&authentikv1alpha1.Outpost{}).
		Owns(&corev1.Secret{}).
		Watches(
			&authentikv1alpha1.OAuth2Provider{},
			handler.EnqueueRequestsFromMapFunc(
				r.outpostsForProvider(authentikv1alpha1.ProviderKindOAuth2)),
		).
		Watches(
			&authentikv1alpha1.SAMLProvider{},
			handler.EnqueueRequestsFromMapFunc(
				r.outpostsForProvider(authentikv1alpha1.ProviderKindSAML)),
		).
		Watches(
			&authentikv1alpha1.ProxyProvider{},
			handler.EnqueueRequestsFromMapFunc(
				r.outpostsForProvider(authentikv1alpha1.ProviderKindProxy)),
		).
		Named("outpost").
		Complete(r)
}

// indexOutpostProviderRefs indexes an Outpost by every provider it references.
func indexOutpostProviderRefs(obj client.Object) []string {
	outpost, ok := obj.(*authentikv1alpha1.Outpost)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(outpost.Spec.ProviderRefs))
	for _, ref := range outpost.Spec.ProviderRefs {
		keys = append(keys, providerRefIndexValue(providerRefKind(ref), ref.Name))
	}
	return keys
}

// outpostsForProvider builds the map function that turns an event on a
// provider of the given kind into reconcile requests for the outposts that
// reference it.
func (r *OutpostReconciler) outpostsForProvider(kind authentikv1alpha1.ProviderKind) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		var list authentikv1alpha1.OutpostList
		err := r.List(ctx, &list,
			client.InNamespace(obj.GetNamespace()),
			client.MatchingFields{
				outpostProviderRefIndexKey: providerRefIndexValue(kind, obj.GetName()),
			},
		)
		if err != nil && !apierrors.IsNotFound(err) {
			log.FromContext(ctx).Error(err, "listing outposts for provider",
				"kind", kind, "provider", obj.GetName())
			return nil
		}

		requests := make([]reconcile.Request, 0, len(list.Items))
		for i := range list.Items {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
		return requests
	}
}

// providerRefKind returns the reference's kind, applying the default that the
// CRD schema would normally have filled in. Defaulting here too keeps the
// index and the resolver agreeing about a reference written before the default
// existed, or built in a test.
func providerRefKind(ref authentikv1alpha1.ProviderReference) authentikv1alpha1.ProviderKind {
	if ref.Kind == "" {
		return authentikv1alpha1.ProviderKindOAuth2
	}
	return ref.Kind
}

// providerRefIndexValue builds the index key for one provider reference. Kind
// is part of the key so that an OAuth2Provider and a SAMLProvider sharing a
// name never wake each other's outposts.
func providerRefIndexValue(kind authentikv1alpha1.ProviderKind, name string) string {
	return string(kind) + "/" + name
}

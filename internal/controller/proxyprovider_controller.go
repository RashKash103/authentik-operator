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
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/recorder"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// ProxyProviderReconciler reconciles a ProxyProvider.
type ProxyProviderReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder recorder.EventRecorder
	Resolver *ConnectionResolver
	// References decides whether references may cross namespaces.
	References ReferencePolicy
}

// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=proxyproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=proxyproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=proxyproviders/finalizers,verbs=update

// Reconcile brings one ProxyProvider in line with authentik.
func (r *ProxyProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var provider authentikv1alpha1.ProxyProvider
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

	adapter := &proxyAdapter{client: akClient, kube: r.Client, refs: ReferenceScope{AuthentikURL: akClient.BaseURL(), Policy: r.References}, provider: &provider}
	outcome, err := Sync(ctx, SyncRequest{
		Object: &provider, Adapter: adapter, Recorder: r.Recorder, Scheme: r.Scheme,
	})
	if err != nil {
		return r.fail(ctx, &provider, err)
	}

	r.recordIdentity(&provider, outcome, adapter)

	MarkReady(&provider.Status.Conditions, provider.Generation)
	if err := r.Status().Update(ctx, &provider); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.V(1).Info("reconciled proxy provider",
		"provider", provider.ProviderName(), "remoteID", outcome.RemoteID,
		"created", outcome.Created, "adopted", outcome.Adopted, "updated", outcome.Updated)
	return ctrl.Result{}, nil
}

// reconcileDelete tears down the authentik provider and releases the finalizer.
func (r *ProxyProviderReconciler) reconcileDelete(
	ctx context.Context,
	provider *authentikv1alpha1.ProxyProvider,
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

	adapter := &proxyAdapter{client: akClient, kube: r.Client, refs: ReferenceScope{AuthentikURL: akClient.BaseURL(), Policy: r.References}, provider: provider}
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

// recordIdentity writes the authentik identity of the provider onto status.
func (r *ProxyProviderReconciler) recordIdentity(
	provider *authentikv1alpha1.ProxyProvider, outcome SyncOutcome, adapter *proxyAdapter,
) {
	status := provider.ManagedStatus()
	status.RemoteID = outcome.RemoteID
	// The scoped name, not the declared one: with a cluster identity set they
	// differ, and status has to report what authentik actually holds.
	status.RemoteName = adapter.DesiredName()
	status.Adopted = status.Adopted || outcome.Adopted
	now := metav1.Now()
	status.LastSyncedTime = &now

	if pk, err := strconv.ParseInt(outcome.RemoteID, 10, 32); err == nil {
		id := int32(pk)
		provider.Status.ProviderID = &id
	}
	if adapter.observed != nil {
		// A proxy provider no outpost serves is unreachable, so the empty
		// list is the diagnostic.
		provider.Status.Outposts = adapter.observed.OutpostSet
	}
}

// fail records a failed reconcile and decides whether to retry.
func (r *ProxyProviderReconciler) fail(
	ctx context.Context, provider *authentikv1alpha1.ProxyProvider, err error,
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
func (r *ProxyProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&authentikv1alpha1.ProxyProvider{}).
		Named("proxyprovider").
		Complete(r)
}

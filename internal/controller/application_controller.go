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

// applicationProviderIndexKey indexes Applications by every provider they
// reference, so that a provider becoming ready can enqueue exactly the
// Applications waiting on it instead of every Application in the cluster.
const applicationProviderIndexKey = ".spec.providerRefs"

// ApplicationReconciler reconciles an Application.
type ApplicationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder recorder.EventRecorder
	Resolver *ConnectionResolver
	// References decides whether references may cross namespaces.
	References ReferencePolicy
}

// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=applications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=applications/finalizers,verbs=update
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=oauth2providers,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile brings one Application in line with authentik.
func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var app authentikv1alpha1.Application
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	akClient, connErr := r.Resolver.Resolve(ctx, app.Spec.ConnectionRef, app.Namespace)

	if !app.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &app, akClient, connErr)
	}

	if connErr != nil {
		return r.fail(ctx, &app, connErr)
	}

	if controllerutil.AddFinalizer(&app, Finalizer) {
		// Persist the finalizer before creating anything remotely. The other
		// order can leak an authentik object: if the process dies after the
		// create, nothing records that cleanup is owed.
		if err := r.Update(ctx, &app); err != nil {
			return ctrl.Result{}, err
		}
	}

	providerID, backchannelIDs, err := r.resolveProviderRefs(ctx, &app)
	if err != nil {
		// An Application applied before its provider is ordinary, not an
		// error condition to give up on: fail() requeues for it.
		return r.fail(ctx, &app, err)
	}

	adapter := &applicationAdapter{
		client:                 akClient,
		app:                    &app,
		providerID:             providerID,
		backchannelProviderIDs: backchannelIDs,
	}
	outcome, err := Sync(ctx, SyncRequest{
		Object: &app, Adapter: adapter, Recorder: r.Recorder, Scheme: r.Scheme,
	})
	if err != nil {
		return r.fail(ctx, &app, err)
	}

	r.recordIdentity(&app, outcome, adapter)

	MarkReady(&app.Status.Conditions, app.Generation)
	if err := r.Status().Update(ctx, &app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.V(1).Info("reconciled application",
		"application", app.ApplicationName(), "slug", app.Spec.Slug,
		"created", outcome.Created, "adopted", outcome.Adopted, "updated", outcome.Updated)
	return ctrl.Result{}, nil
}

// reconcileDelete tears down the authentik application and releases the
// finalizer.
//
// Provider references are deliberately not resolved here: deleting an
// application needs only its slug, and requiring a resolvable provider would
// wedge deletion whenever the provider was removed first.
func (r *ApplicationReconciler) reconcileDelete(
	ctx context.Context,
	app *authentikv1alpha1.Application,
	akClient authentik.Client,
	connErr error,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(app, Finalizer) {
		return ctrl.Result{}, nil
	}

	// Without a usable connection the remote object cannot be removed. Keep
	// retrying rather than dropping the finalizer, which would silently leave
	// an application behind in authentik.
	if connErr != nil {
		MarkNotReady(&app.Status.Conditions, ConnectionProblemReason(connErr),
			"cannot delete the authentik application: "+connErr.Error(), app.Generation)
		if err := r.Status().Update(ctx, app); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		return ctrl.Result{RequeueAfter: retryAfterFailure}, nil
	}

	adapter := &applicationAdapter{client: akClient, app: app}
	done, err := Finalize(ctx, SyncRequest{Object: app, Adapter: adapter, Recorder: r.Recorder})
	if err != nil {
		return ctrl.Result{RequeueAfter: retryAfterFailure}, nil //nolint:nilerr // reported via condition
	}
	if !done {
		return ctrl.Result{RequeueAfter: retryAfterFailure}, nil
	}

	controllerutil.RemoveFinalizer(app, Finalizer)
	return ctrl.Result{}, client.IgnoreNotFound(r.Update(ctx, app))
}

// resolveProviderRefs turns every provider reference into the numeric primary
// key authentik expects.
func (r *ApplicationReconciler) resolveProviderRefs(
	ctx context.Context, app *authentikv1alpha1.Application,
) (primary *int32, backchannel []int32, err error) {
	if app.Spec.ProviderRef != nil {
		primary, err = r.resolveProviderRef(ctx, app.Namespace, "spec.providerRef", *app.Spec.ProviderRef)
		if err != nil {
			return nil, nil, err
		}
	}

	for i, ref := range app.Spec.BackchannelProviderRefs {
		field := fmt.Sprintf("spec.backchannelProviderRefs[%d]", i)
		id, refErr := r.resolveProviderRef(ctx, app.Namespace, field, ref)
		if refErr != nil {
			return nil, nil, refErr
		}
		backchannel = append(backchannel, *id)
	}

	return primary, backchannel, nil
}

// resolveProviderRef reads one provider resource and returns the primary key
// it has recorded in authentik.
//
// Resolution goes through the referenced resource's status.providerID rather
// than looking the name up in authentik. That keeps the Kubernetes objects the
// source of truth: renaming a provider inside authentik cannot repoint the
// application at a different object, and two providers sharing a name in
// authentik cannot make the lookup ambiguous.
func (r *ApplicationReconciler) resolveProviderRef(
	ctx context.Context,
	namespace, field string,
	ref authentikv1alpha1.ProviderReference,
) (*int32, error) {
	key := types.NamespacedName{Name: ref.Name, Namespace: namespace}

	switch ref.EffectiveKind() {
	case authentikv1alpha1.ProviderKindOAuth2:
		var provider authentikv1alpha1.OAuth2Provider
		if err := r.Get(ctx, key, &provider); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, providerRefNotReady(field, ref, "does not exist")
			}
			return nil, err
		}
		if provider.Status.ProviderID == nil {
			return nil, providerRefNotReady(field, ref,
				"has not been created in authentik yet")
		}
		return provider.Status.ProviderID, nil

	case authentikv1alpha1.ProviderKindSAML:
		var provider authentikv1alpha1.SAMLProvider
		if err := r.Get(ctx, key, &provider); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, providerRefNotReady(field, ref, "does not exist")
			}
			return nil, err
		}
		if provider.Status.ProviderID == nil {
			return nil, providerRefNotReady(field, ref,
				"has not been created in authentik yet")
		}
		return provider.Status.ProviderID, nil

	case authentikv1alpha1.ProviderKindProxy:
		var provider authentikv1alpha1.ProxyProvider
		if err := r.Get(ctx, key, &provider); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, providerRefNotReady(field, ref, "does not exist")
			}
			return nil, err
		}
		if provider.Status.ProviderID == nil {
			return nil, providerRefNotReady(field, ref,
				"has not been created in authentik yet")
		}
		return provider.Status.ProviderID, nil

	default:
		return nil, fmt.Errorf("%w: %s: unknown provider kind %q",
			authentik.ErrValidation, field, ref.Kind)
	}
}

// providerRefNotReady builds the error for a reference that cannot be resolved
// yet.
//
// It wraps authentik.ErrNotFound so that ResultFor classifies it as
// ReferenceNotFound and requeues: an Application applied before its provider
// is normal, and must converge once the provider appears rather than failing
// permanently.
func providerRefNotReady(field string, ref authentikv1alpha1.ProviderReference, why string) error {
	return fmt.Errorf("%w: %s: %s %q %s",
		authentik.ErrNotFound, field, ref.EffectiveKind(), ref.Name, why)
}

// recordIdentity writes the authentik identity of the application onto status.
func (r *ApplicationReconciler) recordIdentity(
	app *authentikv1alpha1.Application, outcome SyncOutcome, adapter *applicationAdapter,
) {
	status := app.ManagedStatus()
	status.RemoteID = outcome.RemoteID
	// Applications are deliberately unscoped, so the slug is already what
	// authentik holds; taking it from the adapter keeps the two in step.
	status.RemoteName = adapter.DesiredName()
	status.Adopted = status.Adopted || outcome.Adopted
	now := metav1.Now()
	status.LastSyncedTime = &now

	app.Status.ProviderID = adapter.providerID
	app.Status.BackchannelProviderIDs = adapter.backchannelProviderIDs
	if adapter.observed != nil {
		app.Status.LaunchURL = adapter.observed.GetLaunchUrl()
	}
}

// fail records a failed reconcile and decides whether to retry.
func (r *ApplicationReconciler) fail(
	ctx context.Context, app *authentikv1alpha1.Application, err error,
) (ctrl.Result, error) {
	reason, retry := ResultFor(err)

	if authentik.IsTransient(err) || reason == authentikv1alpha1.ReasonAPIError {
		MarkSyncFailed(&app.Status.Conditions, reason, err.Error(), app.Generation)
	} else {
		MarkNotReady(&app.Status.Conditions, reason, err.Error(), app.Generation)
	}
	app.Status.ObservedGeneration = app.Generation

	if updateErr := r.Status().Update(ctx, app); updateErr != nil {
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

// SetupWithManager registers the controller and the provider watches.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(), &authentikv1alpha1.Application{},
		applicationProviderIndexKey, indexApplicationProviderRefs,
	); err != nil {
		return err
	}

	// Watching the provider kinds is what turns "applied out of order" from a
	// wait into a non-event: an Application whose provider does not exist yet
	// requeues on a backoff, but the provider writing its providerID enqueues
	// the Application immediately.
	return ctrl.NewControllerManagedBy(mgr).
		For(&authentikv1alpha1.Application{}).
		Watches(
			&authentikv1alpha1.OAuth2Provider{},
			handler.EnqueueRequestsFromMapFunc(
				r.applicationsForProvider(authentikv1alpha1.ProviderKindOAuth2)),
		).
		Watches(
			&authentikv1alpha1.SAMLProvider{},
			handler.EnqueueRequestsFromMapFunc(
				r.applicationsForProvider(authentikv1alpha1.ProviderKindSAML)),
		).
		Watches(
			&authentikv1alpha1.ProxyProvider{},
			handler.EnqueueRequestsFromMapFunc(
				r.applicationsForProvider(authentikv1alpha1.ProviderKindProxy)),
		).
		Named("application").
		Complete(r)
}

// indexApplicationProviderRefs returns the index keys for one Application: one
// per provider it references, primary and back-channel alike.
func indexApplicationProviderRefs(obj client.Object) []string {
	app, ok := obj.(*authentikv1alpha1.Application)
	if !ok {
		return nil
	}

	refs := app.ProviderRefs()
	keys := make([]string, 0, len(refs))
	for _, ref := range refs {
		keys = append(keys, ref.Key())
	}
	return keys
}

// applicationsForProvider builds the map function that finds the Applications
// referencing a given provider.
//
// Events are not filtered on the provider having a usable providerID: a
// provider losing its ID, or being deleted outright, is exactly when a
// dependent Application needs to re-report its reference as broken.
func (r *ApplicationReconciler) applicationsForProvider(
	kind authentikv1alpha1.ProviderKind,
) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		ref := authentikv1alpha1.ProviderReference{Kind: kind, Name: obj.GetName()}

		var list authentikv1alpha1.ApplicationList
		err := r.List(ctx, &list,
			client.InNamespace(obj.GetNamespace()),
			client.MatchingFields{applicationProviderIndexKey: ref.Key()},
		)
		if err != nil && !apierrors.IsNotFound(err) {
			log.FromContext(ctx).Error(err, "listing applications for provider",
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

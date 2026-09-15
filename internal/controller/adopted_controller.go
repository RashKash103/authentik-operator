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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// adoptedRefreshInterval re-resolves an adopted object periodically.
//
// These resources adopt something the operator does not own, so it can be
// renamed or deleted in authentik without any event reaching us. Re-resolving
// is how a provider finds out its flow disappeared, rather than failing later
// with a stale UUID.
const adoptedRefreshInterval = 10 * time.Minute

// FlowReconciler resolves a Flow to the authentik flow it names.
type FlowReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Resolver *ConnectionResolver
}

// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=flows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=flows/status,verbs=get;update;patch

// Reconcile resolves the flow slug and records its UUID.
func (r *FlowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var flow authentikv1alpha1.Flow
	if err := r.Get(ctx, req.NamespacedName, &flow); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !flow.DeletionTimestamp.IsZero() {
		// Nothing was created in authentik, so there is nothing to clean up
		// and no finalizer to hold.
		return ctrl.Result{}, nil
	}

	return r.resolve(ctx, &flow, &flow.Status, flow.Spec.ConnectionRef, flow.Spec.ExistingSlug,
		func(c authentik.Client, id string) (string, error) { return c.ResolveFlow(ctx, id) })
}

// resolve is the body every adopt-only kind shares.
func (r *FlowReconciler) resolve(
	ctx context.Context,
	obj client.Object,
	status *authentikv1alpha1.AdoptedObjectStatus,
	connRef authentikv1alpha1.ConnectionReference,
	identifier string,
	lookup func(authentik.Client, string) (string, error),
) (ctrl.Result, error) {
	return resolveAdopted(ctx, r.Client, r.Resolver, obj, status, connRef, identifier, lookup)
}

// SetupWithManager registers the controller.
func (r *FlowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&authentikv1alpha1.Flow{}).
		Named("flow").
		Complete(r)
}

// PropertyMappingReconciler resolves a PropertyMapping.
type PropertyMappingReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Resolver *ConnectionResolver
}

// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=propertymappings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=propertymappings/status,verbs=get;update;patch

// Reconcile resolves the property mapping name and records its UUID.
func (r *PropertyMappingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var mapping authentikv1alpha1.PropertyMapping
	if err := r.Get(ctx, req.NamespacedName, &mapping); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !mapping.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	return resolveAdopted(ctx, r.Client, r.Resolver, &mapping, &mapping.Status,
		mapping.Spec.ConnectionRef, mapping.Spec.ExistingName,
		func(c authentik.Client, id string) (string, error) { return c.ResolvePropertyMapping(ctx, id) })
}

// SetupWithManager registers the controller.
func (r *PropertyMappingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&authentikv1alpha1.PropertyMapping{}).
		Named("propertymapping").
		Complete(r)
}

// CertificateKeyPairReconciler resolves a CertificateKeyPair.
type CertificateKeyPairReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Resolver *ConnectionResolver
}

// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=certificatekeypairs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=certificatekeypairs/status,verbs=get;update;patch

// Reconcile resolves the key pair name and records its UUID.
func (r *CertificateKeyPairReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pair authentikv1alpha1.CertificateKeyPair
	if err := r.Get(ctx, req.NamespacedName, &pair); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !pair.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	return resolveAdopted(ctx, r.Client, r.Resolver, &pair, &pair.Status,
		pair.Spec.ConnectionRef, pair.Spec.ExistingName,
		func(c authentik.Client, id string) (string, error) {
			return c.ResolveCertificateKeyPair(ctx, id)
		})
}

// SetupWithManager registers the controller.
func (r *CertificateKeyPairReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&authentikv1alpha1.CertificateKeyPair{}).
		Named("certificatekeypair").
		Complete(r)
}

// resolveAdopted looks an object up in authentik and records what it found.
//
// These kinds adopt rather than create, so there is no finalizer, no drift
// correction and no deletion path — only the question "does this still resolve,
// and to what".
func resolveAdopted(
	ctx context.Context,
	c client.Client,
	resolver *ConnectionResolver,
	obj client.Object,
	status *authentikv1alpha1.AdoptedObjectStatus,
	connRef authentikv1alpha1.ConnectionReference,
	identifier string,
	lookup func(authentik.Client, string) (string, error),
) (ctrl.Result, error) {
	status.ObservedGeneration = obj.GetGeneration()

	akClient, err := resolver.Resolve(ctx, connRef, obj.GetNamespace())
	if err != nil {
		SetCondition(&status.Conditions, authentikv1alpha1.ConditionReady, metav1.ConditionFalse,
			ConnectionProblemReason(err), err.Error(), obj.GetGeneration())
		return finishAdopted(ctx, c, obj, retryAfterFailure)
	}

	remoteID, err := lookup(akClient, identifier)
	if err != nil {
		reason, retry := ResultFor(err)
		SetCondition(&status.Conditions, authentikv1alpha1.ConditionReady, metav1.ConditionFalse,
			reason, err.Error(), obj.GetGeneration())
		status.RemoteID = ""
		if !retry {
			log.FromContext(ctx).Info("not retrying until the spec changes",
				"reason", reason, "error", err.Error())
			return finishAdopted(ctx, c, obj, 0)
		}
		return finishAdopted(ctx, c, obj, retryAfterFailure)
	}

	status.RemoteID = remoteID
	status.RemoteName = identifier
	now := metav1.Now()
	status.LastSyncedTime = &now
	SetCondition(&status.Conditions, authentikv1alpha1.ConditionReady, metav1.ConditionTrue,
		authentikv1alpha1.ReasonSucceeded, "Resolved in authentik", obj.GetGeneration())

	return finishAdopted(ctx, c, obj, adoptedRefreshInterval)
}

// finishAdopted writes status and schedules the next look.
func finishAdopted(
	ctx context.Context, c client.Client, obj client.Object, requeue time.Duration,
) (ctrl.Result, error) {
	if err := c.Status().Update(ctx, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return ctrl.Result{RequeueAfter: requeue}, nil
}

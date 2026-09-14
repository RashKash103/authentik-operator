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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// tokenSecretIndexKey indexes connections by the Secret they read credentials
// from, so a rotated token takes effect without waiting for the next probe or
// an operator restart.
const tokenSecretIndexKey = ".spec.tokenSecretRef.name"

// AuthentikConnectionReconciler reconciles a namespaced AuthentikConnection.
type AuthentikConnectionReconciler struct {
	client.Client
	Resolver *ConnectionResolver
}

// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=authentikconnections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=authentikconnections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=authentikconnections/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile probes the referenced authentik instance and records what it found.
func (r *AuthentikConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var conn authentikv1alpha1.AuthentikConnection
	if err := r.Get(ctx, req.NamespacedName, &conn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// The connection owns no remote state, so deletion needs no finalizer:
	// there is nothing in authentik to clean up.
	if !conn.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	tokenRef := authentikv1alpha1.SecretKeyReference{
		Name:      conn.Spec.TokenSecretRef.Name,
		Namespace: conn.Namespace,
		Key:       conn.Spec.TokenSecretRef.Key,
	}
	var caRef *authentikv1alpha1.SecretKeyReference
	if conn.Spec.CABundleSecretRef != nil {
		caRef = &authentikv1alpha1.SecretKeyReference{
			Name:      conn.Spec.CABundleSecretRef.Name,
			Namespace: conn.Namespace,
			Key:       conn.Spec.CABundleSecretRef.Key,
		}
	}

	result := probeConnection(ctx, func() (authentik.Client, error) {
		return r.Resolver.build(ctx, conn.Spec.ConnectionSettings, tokenRef, caRef)
	}, conn.Spec.ConnectionSettings, conn.Generation, conn.Status)

	conn.Status = result.status
	if err := r.Status().Update(ctx, &conn); err != nil {
		// A conflict means someone else wrote first; reconciling again is
		// cheaper and clearer than merging status by hand.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.V(1).Info("probed authentik instance",
		"url", conn.Spec.URL,
		"version", result.status.AuthentikVersion,
		"ready", IsReady(result.status.Conditions))

	return ctrl.Result{RequeueAfter: result.requeue}, result.probeErr
}

// SetupWithManager registers the controller and the Secret watch.
func (r *AuthentikConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	idx := func(obj client.Object) []string {
		conn, ok := obj.(*authentikv1alpha1.AuthentikConnection)
		if !ok {
			return nil
		}
		names := []string{conn.Spec.TokenSecretRef.Name}
		if conn.Spec.CABundleSecretRef != nil {
			names = append(names, conn.Spec.CABundleSecretRef.Name)
		}
		return names
	}
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(), &authentikv1alpha1.AuthentikConnection{}, tokenSecretIndexKey, idx,
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&authentikv1alpha1.AuthentikConnection{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.connectionsForSecret),
			builder.WithPredicates(secretDataChanged()),
		).
		Named("authentikconnection").
		Complete(r)
}

// connectionsForSecret finds the connections in a Secret's namespace that read
// credentials from it.
func (r *AuthentikConnectionReconciler) connectionsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	var list authentikv1alpha1.AuthentikConnectionList
	err := r.List(ctx, &list,
		client.InNamespace(obj.GetNamespace()),
		client.MatchingFields{tokenSecretIndexKey: obj.GetName()},
	)
	if err != nil && !apierrors.IsNotFound(err) {
		log.FromContext(ctx).Error(err, "listing connections for secret", "secret", obj.GetName())
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

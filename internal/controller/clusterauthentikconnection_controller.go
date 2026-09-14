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
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// clusterTokenSecretIndexKey indexes cluster connections by the fully qualified
// "namespace/name" of the Secrets they read.
//
// Unlike the namespaced kind, a cluster connection can reference a Secret in
// any namespace, so the index key has to carry the namespace or a Secret named
// "authentik-token" in one namespace would wake connections pointing at an
// unrelated Secret of the same name elsewhere.
const clusterTokenSecretIndexKey = ".spec.tokenSecretRef.namespacedName"

// ClusterAuthentikConnectionReconciler reconciles a ClusterAuthentikConnection.
type ClusterAuthentikConnectionReconciler struct {
	client.Client
	Resolver *ConnectionResolver
}

// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=clusterauthentikconnections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=clusterauthentikconnections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=clusterauthentikconnections/finalizers,verbs=update

// Reconcile probes the referenced authentik instance and records what it found.
func (r *ClusterAuthentikConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var conn authentikv1alpha1.ClusterAuthentikConnection
	if err := r.Get(ctx, req.NamespacedName, &conn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !conn.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	result := probeConnection(ctx, func() (authentik.Client, error) {
		return r.Resolver.build(ctx, conn.Spec.ConnectionSettings, conn.Spec.TokenSecretRef, conn.Spec.CABundleSecretRef)
	}, conn.Spec.ConnectionSettings, conn.Generation, conn.Status)

	conn.Status = result.status
	if err := r.Status().Update(ctx, &conn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.V(1).Info("probed authentik instance",
		"url", conn.Spec.URL,
		"version", result.status.AuthentikVersion,
		"ready", IsReady(result.status.Conditions))

	return ctrl.Result{RequeueAfter: result.requeue}, result.probeErr
}

// SetupWithManager registers the controller and the Secret watch.
func (r *ClusterAuthentikConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	idx := func(obj client.Object) []string {
		conn, ok := obj.(*authentikv1alpha1.ClusterAuthentikConnection)
		if !ok {
			return nil
		}
		keys := []string{secretIndexValue(conn.Spec.TokenSecretRef.Namespace, conn.Spec.TokenSecretRef.Name)}
		if ref := conn.Spec.CABundleSecretRef; ref != nil {
			keys = append(keys, secretIndexValue(ref.Namespace, ref.Name))
		}
		return keys
	}
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(), &authentikv1alpha1.ClusterAuthentikConnection{}, clusterTokenSecretIndexKey, idx,
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&authentikv1alpha1.ClusterAuthentikConnection{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.connectionsForSecret),
			builder.WithPredicates(secretDataChanged()),
		).
		Named("clusterauthentikconnection").
		Complete(r)
}

func (r *ClusterAuthentikConnectionReconciler) connectionsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	var list authentikv1alpha1.ClusterAuthentikConnectionList
	err := r.List(ctx, &list, client.MatchingFields{
		clusterTokenSecretIndexKey: secretIndexValue(obj.GetNamespace(), obj.GetName()),
	})
	if err != nil && !apierrors.IsNotFound(err) {
		log.FromContext(ctx).Error(err, "listing cluster connections for secret", "secret", obj.GetName())
		return nil
	}

	requests := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: list.Items[i].Name},
		})
	}
	return requests
}

// secretIndexValue builds the "namespace/name" index key for a Secret.
func secretIndexValue(namespace, name string) string {
	return namespace + "/" + name
}

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/recorder"
	"sigs.k8s.io/yaml"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// KubernetesServiceConnectionReconciler reconciles a
// KubernetesServiceConnection.
type KubernetesServiceConnectionReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder recorder.EventRecorder
	Resolver *ConnectionResolver
}

// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=kubernetesserviceconnections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=kubernetesserviceconnections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=kubernetesserviceconnections/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile brings one KubernetesServiceConnection in line with authentik.
func (r *KubernetesServiceConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var conn authentikv1alpha1.KubernetesServiceConnection
	if err := r.Get(ctx, req.NamespacedName, &conn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	akClient, connErr := r.Resolver.Resolve(ctx, conn.Spec.ConnectionRef, conn.Namespace)

	if !conn.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &conn, akClient, connErr)
	}

	if connErr != nil {
		return r.fail(ctx, &conn, connErr)
	}

	if controllerutil.AddFinalizer(&conn, Finalizer) {
		// Persist the finalizer before creating anything remotely. The other
		// order can leak an authentik object: if the process dies after the
		// create, nothing records that cleanup is owed.
		if err := r.Update(ctx, &conn); err != nil {
			return ctrl.Result{}, err
		}
	}

	kubeconfig, err := r.desiredKubeconfig(ctx, &conn)
	if err != nil {
		return r.fail(ctx, &conn, err)
	}

	adapter := &kubernetesServiceConnectionAdapter{client: akClient, conn: &conn, kubeconfig: kubeconfig}
	outcome, err := Sync(ctx, SyncRequest{
		Object: &conn, Adapter: adapter, Recorder: r.Recorder, Scheme: r.Scheme,
	})
	if err != nil {
		return r.fail(ctx, &conn, err)
	}

	recordServiceConnectionIdentity(&conn.Status.ServiceConnectionStatus, outcome, adapter.DesiredName(), akClient.BaseURL())

	MarkReady(&conn.Status.Conditions, conn.Generation)
	if err := r.Status().Update(ctx, &conn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.V(1).Info("reconciled kubernetes service connection",
		"serviceConnection", conn.ServiceConnectionName(), "remoteID", outcome.RemoteID,
		"created", outcome.Created, "adopted", outcome.Adopted, "updated", outcome.Updated)
	return ctrl.Result{}, nil
}

// reconcileDelete tears down the authentik connection and releases the
// finalizer.
func (r *KubernetesServiceConnectionReconciler) reconcileDelete(
	ctx context.Context,
	conn *authentikv1alpha1.KubernetesServiceConnection,
	akClient authentik.Client,
	connErr error,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(conn, Finalizer) {
		return ctrl.Result{}, nil
	}

	// Without a usable connection the remote object cannot be removed. Keep
	// retrying rather than dropping the finalizer, which would silently leave
	// a service connection behind in authentik.
	if connErr != nil {
		MarkNotReady(&conn.Status.Conditions, ConnectionProblemReason(connErr),
			"cannot delete the authentik service connection: "+connErr.Error(), conn.Generation)
		if err := r.Status().Update(ctx, conn); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		return ctrl.Result{RequeueAfter: retryAfterFailure}, nil
	}

	adapter := &kubernetesServiceConnectionAdapter{client: akClient, conn: conn}
	done, err := Finalize(ctx, SyncRequest{Object: conn, Adapter: adapter, Recorder: r.Recorder})
	if err != nil {
		return ctrl.Result{RequeueAfter: retryAfterFailure}, nil //nolint:nilerr // reported via condition
	}
	if !done {
		return ctrl.Result{RequeueAfter: retryAfterFailure}, nil
	}

	controllerutil.RemoveFinalizer(conn, Finalizer)
	return ctrl.Result{}, client.IgnoreNotFound(r.Update(ctx, conn))
}

// desiredKubeconfig reads and parses the referenced kubeconfig, if there is
// one.
func (r *KubernetesServiceConnectionReconciler) desiredKubeconfig(
	ctx context.Context, conn *authentikv1alpha1.KubernetesServiceConnection,
) (map[string]any, error) {
	ref := conn.Spec.KubeconfigSecretRef
	if ref == nil {
		// A local connection; authentik uses its own service account.
		return nil, nil
	}

	var secret corev1.Secret
	key := types.NamespacedName{Name: ref.Name, Namespace: conn.Namespace}
	if err := r.Get(ctx, key, &secret); err != nil {
		return nil, fmt.Errorf("%w: reading kubeconfigSecretRef: %s", ErrCredentialsUnavailable, key)
	}
	value, ok := secret.Data[ref.Key]
	if !ok || len(value) == 0 {
		return nil, fmt.Errorf("%w: Secret %s has no non-empty key %q", ErrCredentialsUnavailable, key, ref.Key)
	}

	return parseKubeconfig(value, key, ref.Key)
}

// SetupWithManager registers the controller.
func (r *KubernetesServiceConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&authentikv1alpha1.KubernetesServiceConnection{}).
		Named("kubernetesserviceconnection").
		Complete(r)
}

// fail records a failed reconcile and decides whether to retry.
func (r *KubernetesServiceConnectionReconciler) fail(
	ctx context.Context, conn *authentikv1alpha1.KubernetesServiceConnection, err error,
) (ctrl.Result, error) {
	reason, retry := ResultFor(err)
	markReconcileFailure(&conn.Status.Conditions, reason, err, conn.Generation)
	conn.Status.ObservedGeneration = conn.Generation

	if updateErr := r.Status().Update(ctx, conn); updateErr != nil {
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

// DockerServiceConnectionReconciler reconciles a DockerServiceConnection.
type DockerServiceConnectionReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder recorder.EventRecorder
	Resolver *ConnectionResolver
	// References decides whether references may cross namespaces.
	References ReferencePolicy
}

// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=dockerserviceconnections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=dockerserviceconnections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=authentik.k8s.rka.sh,resources=dockerserviceconnections/finalizers,verbs=update

// Reconcile brings one DockerServiceConnection in line with authentik.
func (r *DockerServiceConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var conn authentikv1alpha1.DockerServiceConnection
	if err := r.Get(ctx, req.NamespacedName, &conn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	akClient, connErr := r.Resolver.Resolve(ctx, conn.Spec.ConnectionRef, conn.Namespace)

	if !conn.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &conn, akClient, connErr)
	}

	if connErr != nil {
		return r.fail(ctx, &conn, connErr)
	}

	if controllerutil.AddFinalizer(&conn, Finalizer) {
		if err := r.Update(ctx, &conn); err != nil {
			return ctrl.Result{}, err
		}
	}

	adapter := &dockerServiceConnectionAdapter{client: akClient, kube: r.Client, refs: ReferenceScope{AuthentikURL: akClient.BaseURL(), Policy: r.References}, conn: &conn}
	outcome, err := Sync(ctx, SyncRequest{
		Object: &conn, Adapter: adapter, Recorder: r.Recorder, Scheme: r.Scheme,
	})
	if err != nil {
		return r.fail(ctx, &conn, err)
	}

	recordServiceConnectionIdentity(&conn.Status.ServiceConnectionStatus, outcome, adapter.DesiredName(), akClient.BaseURL())

	MarkReady(&conn.Status.Conditions, conn.Generation)
	if err := r.Status().Update(ctx, &conn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.V(1).Info("reconciled docker service connection",
		"serviceConnection", conn.ServiceConnectionName(), "remoteID", outcome.RemoteID,
		"created", outcome.Created, "adopted", outcome.Adopted, "updated", outcome.Updated)
	return ctrl.Result{}, nil
}

// reconcileDelete tears down the authentik connection and releases the
// finalizer.
func (r *DockerServiceConnectionReconciler) reconcileDelete(
	ctx context.Context,
	conn *authentikv1alpha1.DockerServiceConnection,
	akClient authentik.Client,
	connErr error,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(conn, Finalizer) {
		return ctrl.Result{}, nil
	}

	if connErr != nil {
		MarkNotReady(&conn.Status.Conditions, ConnectionProblemReason(connErr),
			"cannot delete the authentik service connection: "+connErr.Error(), conn.Generation)
		if err := r.Status().Update(ctx, conn); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		return ctrl.Result{RequeueAfter: retryAfterFailure}, nil
	}

	adapter := &dockerServiceConnectionAdapter{client: akClient, kube: r.Client, refs: ReferenceScope{AuthentikURL: akClient.BaseURL(), Policy: r.References}, conn: conn}
	done, err := Finalize(ctx, SyncRequest{Object: conn, Adapter: adapter, Recorder: r.Recorder})
	if err != nil {
		return ctrl.Result{RequeueAfter: retryAfterFailure}, nil //nolint:nilerr // reported via condition
	}
	if !done {
		return ctrl.Result{RequeueAfter: retryAfterFailure}, nil
	}

	controllerutil.RemoveFinalizer(conn, Finalizer)
	return ctrl.Result{}, client.IgnoreNotFound(r.Update(ctx, conn))
}

// SetupWithManager registers the controller.
func (r *DockerServiceConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&authentikv1alpha1.DockerServiceConnection{}).
		Named("dockerserviceconnection").
		Complete(r)
}

// fail records a failed reconcile and decides whether to retry.
func (r *DockerServiceConnectionReconciler) fail(
	ctx context.Context, conn *authentikv1alpha1.DockerServiceConnection, err error,
) (ctrl.Result, error) {
	reason, retry := ResultFor(err)
	markReconcileFailure(&conn.Status.Conditions, reason, err, conn.Generation)
	conn.Status.ObservedGeneration = conn.Generation

	if updateErr := r.Status().Update(ctx, conn); updateErr != nil {
		return ctrl.Result{}, client.IgnoreNotFound(updateErr)
	}
	if !retry {
		log.FromContext(ctx).Info("not retrying until the spec changes", "reason", reason, "error", err.Error())
		return ctrl.Result{}, nil
	}
	return ctrl.Result{RequeueAfter: retryAfterFailure}, nil
}

// parseKubeconfig decodes a kubeconfig into the generic map authentik stores
// it as.
//
// A kubeconfig that will not parse is reported as a missing credential rather
// than a rejected spec. The difference is whether the resource retries: the
// fix is an edit to a Secret, and a Secret edit produces no event on this
// resource, so a resource that stopped retrying would stay broken until the
// operator restarted.
//
// The parser's own error is deliberately dropped. YAML errors quote the line
// that failed, and that line may be a client certificate or a bearer token.
func parseKubeconfig(data []byte, secret types.NamespacedName, key string) (map[string]any, error) {
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil || parsed == nil {
		return nil, fmt.Errorf("%w: Secret %s key %q does not hold a valid kubeconfig",
			ErrCredentialsUnavailable, secret, key)
	}
	return parsed, nil
}

// recordServiceConnectionIdentity writes the authentik identity of a service
// connection onto its status.
// name is the scoped name the adapter uses, not the declared one: with a
// cluster identity set they differ, and status reports what authentik holds.
func recordServiceConnectionIdentity(
	status *authentikv1alpha1.ServiceConnectionStatus, outcome SyncOutcome, name, authentikURL string,
) {
	status.RemoteID = outcome.RemoteID
	status.RemoteName = name
	status.AuthentikURL = authentikURL
	status.Adopted = status.Adopted || outcome.Adopted
	status.ServiceConnectionID = outcome.RemoteID
	now := metav1.Now()
	status.LastSyncedTime = &now
}

// markReconcileFailure records a failed reconcile on the conditions. It is
// shared by every reconciler in this file and by the Outpost reconciler.
//
// Failing to reach the API and failing to match the spec are different
// questions, so they are reported on different conditions; conflating them
// hides which one actually broke.
func markReconcileFailure(
	conditions *[]metav1.Condition, reason string, err error, generation int64,
) {
	if authentik.IsTransient(err) || reason == authentikv1alpha1.ReasonAPIError {
		MarkSyncFailed(conditions, reason, err.Error(), generation)
		return
	}
	MarkNotReady(conditions, reason, err.Error(), generation)
}

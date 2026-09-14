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

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/recorder"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// Finalizer is added to every resource that owns an object inside authentik.
const Finalizer = "authentik.k8s.rka.sh/finalizer"

// ManagedObject is implemented by every custom resource that manages one
// object inside authentik.
type ManagedObject interface {
	client.Object

	// ConnectionRef identifies the authentik instance to act on.
	ConnectionRef() authentikv1alpha1.ConnectionReference
	// DeletionPolicy says what to do with the remote object on delete.
	DeletionPolicy() authentikv1alpha1.DeletionPolicy
	// AdoptionPolicy says what to do when a same-named object already exists.
	AdoptionPolicy() authentikv1alpha1.AdoptionPolicy
	// ManagedStatus exposes the embedded status for the engine to write.
	ManagedStatus() *authentikv1alpha1.ManagedResourceStatus
}

// RemoteAdapter is the per-kind authentik-side implementation. One is built
// per reconcile, bound to a client and the desired spec.
//
// The engine owns the lifecycle; an adapter only has to know how to talk to
// one endpoint. That keeps adoption, drift and deletion semantics identical
// across every resource kind rather than reimplemented six times.
type RemoteAdapter interface {
	// Kind names the authentik object type, for messages and events.
	Kind() string

	// DesiredName is the name or slug authentik keys the object by. It is what
	// a pre-existing object would collide on.
	DesiredName() string

	// Exists reports whether the object with this remote ID is still present.
	Exists(ctx context.Context, id string) (bool, error)

	// FindByName returns the remote ID of an object with the desired name, or
	// an error satisfying authentik.IsNotFound when there is none.
	FindByName(ctx context.Context) (string, error)

	// Create makes the object and returns its remote ID.
	Create(ctx context.Context) (string, error)

	// Update reconciles the remote object towards the spec, reporting whether
	// anything actually changed.
	Update(ctx context.Context, id string) (bool, error)

	// Delete removes the object. Deleting something already gone must succeed.
	Delete(ctx context.Context, id string) error
}

// SyncRequest carries everything the engine needs for one reconcile.
type SyncRequest struct {
	Object   ManagedObject
	Adapter  RemoteAdapter
	Recorder recorder.EventRecorder
	Scheme   *runtime.Scheme
}

// SyncOutcome describes what the engine did, so a caller can log or requeue.
type SyncOutcome struct {
	// RemoteID is the authentik identifier now recorded in status.
	RemoteID string
	// Created is true when this reconcile created the remote object.
	Created bool
	// Adopted is true when this reconcile took over a pre-existing object.
	Adopted bool
	// Updated is true when drift was corrected.
	Updated bool
}

// Sync drives one resource through create, adopt, drift-correct or delete.
//
// It never writes status itself: the caller owns the object and performs a
// single status update after Sync returns, so a partially-applied status can
// never be left behind by an early return.
func Sync(ctx context.Context, req SyncRequest) (SyncOutcome, error) {
	if !req.Object.GetDeletionTimestamp().IsZero() {
		return SyncOutcome{}, nil
	}
	return ensureRemote(ctx, req)
}

// ensureRemote brings the authentik object in line with the spec.
func ensureRemote(ctx context.Context, req SyncRequest) (SyncOutcome, error) {
	logger := log.FromContext(ctx)
	status := req.Object.ManagedStatus()

	// A recorded remote ID is authoritative. authentik keys objects by
	// name/slug, and either side may be renamed, so looking up by ID first is
	// what stops a rename turning into a duplicate object.
	if status.RemoteID != "" {
		exists, err := req.Adapter.Exists(ctx, status.RemoteID)
		if err != nil {
			return SyncOutcome{}, err
		}
		if exists {
			changed, err := req.Adapter.Update(ctx, status.RemoteID)
			if err != nil {
				return SyncOutcome{}, err
			}
			if changed {
				req.recordf(corev1.EventTypeNormal, "DriftCorrected",
					"Updated %s %q in authentik to match spec", req.Adapter.Kind(), req.Adapter.DesiredName())
			}
			return SyncOutcome{RemoteID: status.RemoteID, Updated: changed}, nil
		}

		// The object was deleted out from under us. Recreate it rather than
		// wedging: the spec is still the declared intent.
		logger.Info("recorded authentik object is gone, recreating",
			"kind", req.Adapter.Kind(), "remoteID", status.RemoteID)
		req.recordf(corev1.EventTypeWarning, "RemoteObjectMissing",
			"%s %q vanished from authentik and is being recreated", req.Adapter.Kind(), req.Adapter.DesiredName())
	}

	// No usable ID: either first reconcile, or the object was deleted remotely.
	existingID, err := req.Adapter.FindByName(ctx)
	switch {
	case err == nil:
		return adopt(ctx, req, existingID)
	case authentik.IsNotFound(err):
		// Nothing there; create it below.
	default:
		return SyncOutcome{}, err
	}

	id, err := req.Adapter.Create(ctx)
	if err != nil {
		return SyncOutcome{}, err
	}
	req.recordf(corev1.EventTypeNormal, "Created",
		"Created %s %q in authentik", req.Adapter.Kind(), req.Adapter.DesiredName())
	return SyncOutcome{RemoteID: id, Created: true}, nil
}

// adopt handles a pre-existing authentik object with the desired name.
func adopt(ctx context.Context, req SyncRequest, existingID string) (SyncOutcome, error) {
	status := req.Object.ManagedStatus()

	// Already ours: the ID simply was not recorded yet, for instance because a
	// previous reconcile crashed between creating and writing status.
	wasOurs := status.RemoteID == existingID

	if !wasOurs && req.Object.AdoptionPolicy() != authentikv1alpha1.AdoptionPolicyAdoptExisting {
		return SyncOutcome{}, &AdoptionConflictError{
			Kind: req.Adapter.Kind(),
			Name: req.Adapter.DesiredName(),
			ID:   existingID,
		}
	}

	changed, err := req.Adapter.Update(ctx, existingID)
	if err != nil {
		return SyncOutcome{}, err
	}
	if !wasOurs {
		req.recordf(corev1.EventTypeNormal, "Adopted",
			"Adopted pre-existing %s %q in authentik", req.Adapter.Kind(), req.Adapter.DesiredName())
	}
	return SyncOutcome{RemoteID: existingID, Adopted: !wasOurs, Updated: changed}, nil
}

// Finalize removes the authentik object when the resource is being deleted.
//
// It returns true once the finalizer may be removed.
func Finalize(ctx context.Context, req SyncRequest) (bool, error) {
	logger := log.FromContext(ctx)
	status := req.Object.ManagedStatus()

	if req.Object.DeletionPolicy() == authentikv1alpha1.DeletionPolicyOrphan {
		logger.Info("leaving authentik object in place per deletionPolicy",
			"kind", req.Adapter.Kind(), "remoteID", status.RemoteID)
		req.recordf(corev1.EventTypeNormal, "Orphaned",
			"Left %s %q in authentik per deletionPolicy: Orphan", req.Adapter.Kind(), req.Adapter.DesiredName())
		return true, nil
	}

	if status.RemoteID == "" {
		// Nothing was ever created, so there is nothing to clean up.
		return true, nil
	}

	if err := req.Adapter.Delete(ctx, status.RemoteID); err != nil {
		// Already gone is success. Treating it as an error would wedge the
		// resource and, with it, deletion of its whole namespace.
		if authentik.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}

	req.recordf(corev1.EventTypeNormal, "Deleted",
		"Deleted %s %q from authentik", req.Adapter.Kind(), req.Adapter.DesiredName())
	return true, nil
}

// EnsureFinalizer adds the finalizer if it is missing, reporting whether the
// object needs to be persisted.
func EnsureFinalizer(obj client.Object) bool {
	return controllerutil.AddFinalizer(obj, Finalizer)
}

// RemoveFinalizer drops the finalizer, reporting whether anything changed.
func RemoveFinalizer(obj client.Object) bool {
	return controllerutil.RemoveFinalizer(obj, Finalizer)
}

// AdoptionConflictError reports that an authentik object with the desired name
// already exists and the resource is not permitted to take it over.
type AdoptionConflictError struct {
	Kind string
	Name string
	ID   string
}

func (e *AdoptionConflictError) Error() string {
	return fmt.Sprintf(
		"%s %q already exists in authentik (id %s) and was not created by this resource; "+
			"set spec.adoptionPolicy to AdoptExisting to manage it, or choose a different name",
		e.Kind, e.Name, e.ID)
}

// IsAdoptionConflict reports whether err is an adoption conflict.
func IsAdoptionConflict(err error) bool {
	var target *AdoptionConflictError
	return errors.As(err, &target)
}

// recordf emits a Kubernetes Event when a recorder is configured.
//
// The events API takes a "related" object and an "action" alongside the
// reason; there is no second object involved here, and the reason doubles as
// the action because each one already names exactly what was done.
func (r SyncRequest) recordf(eventType, reason, format string, args ...any) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(r.Object, nil, eventType, reason, reason, format, args...)
}

// ResultFor maps a sync error to the condition reason and requeue behaviour a
// controller should apply.
//
// The distinction that matters is "not yet" versus "never": a missing
// reference will resolve once someone creates it and should be retried, while
// an ambiguous reference or a rejected spec will not fix itself and should not
// spin.
func ResultFor(err error) (reason string, requeue bool) {
	switch {
	case err == nil:
		return authentikv1alpha1.ReasonSucceeded, false
	case IsAdoptionConflict(err):
		return authentikv1alpha1.ReasonAdoptionConflict, false
	case IsSecretOwnershipError(err):
		// The Secret belongs to something else and will not start belonging to
		// us on its own; retrying would just overwrite it later.
		return authentikv1alpha1.ReasonSecretConflict, false
	case authentik.IsAmbiguous(err):
		return authentikv1alpha1.ReasonReferenceAmbiguous, false
	case authentik.IsValidation(err):
		return authentikv1alpha1.ReasonInvalidSpec, false
	case authentik.IsNotFound(err):
		return authentikv1alpha1.ReasonReferenceNotFound, true
	case errors.Is(err, ErrConnectionNotFound), errors.Is(err, ErrCredentialsUnavailable):
		return authentikv1alpha1.ReasonReferenceNotFound, true
	case errors.Is(err, ErrConnectionNotReady), errors.Is(err, ErrNamespaceNotAllowed):
		return authentikv1alpha1.ReasonConnectionNotReady, true
	default:
		return authentikv1alpha1.ReasonAPIError, true
	}
}

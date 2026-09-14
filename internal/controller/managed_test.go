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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// fakeManaged is a stand-in for a real custom resource.
type fakeManaged struct {
	authentikv1alpha1.AuthentikConnection // supplies client.Object

	deletion authentikv1alpha1.DeletionPolicy
	adoption authentikv1alpha1.AdoptionPolicy
	managed  authentikv1alpha1.ManagedResourceStatus
}

func (f *fakeManaged) ConnectionRef() authentikv1alpha1.ConnectionReference {
	return authentikv1alpha1.ConnectionReference{Name: "conn"}
}
func (f *fakeManaged) DeletionPolicy() authentikv1alpha1.DeletionPolicy { return f.deletion }
func (f *fakeManaged) AdoptionPolicy() authentikv1alpha1.AdoptionPolicy { return f.adoption }
func (f *fakeManaged) ManagedStatus() *authentikv1alpha1.ManagedResourceStatus {
	return &f.managed
}

func newFake(remoteID string) *fakeManaged {
	f := &fakeManaged{
		deletion: authentikv1alpha1.DeletionPolicyDelete,
		adoption: authentikv1alpha1.AdoptionPolicyFailOnConflict,
	}
	f.Name = "sample"
	f.Namespace = "default"
	f.managed.RemoteID = remoteID
	return f
}

// fakeAdapter records calls so a test can assert on the lifecycle taken.
type fakeAdapter struct {
	existsResult bool
	existsErr    error
	findID       string
	findErr      error
	createID     string
	createErr    error
	updateChange bool
	updateErr    error
	deleteErr    error

	created, updated, deleted int
	updatedID, deletedID      string
}

func (a *fakeAdapter) Kind() string        { return "Application" }
func (a *fakeAdapter) DesiredName() string { return "grafana" }
func (a *fakeAdapter) Exists(context.Context, string) (bool, error) {
	return a.existsResult, a.existsErr
}
func (a *fakeAdapter) FindByName(context.Context) (string, error) {
	return a.findID, a.findErr
}
func (a *fakeAdapter) Create(context.Context) (string, error) {
	a.created++
	return a.createID, a.createErr
}
func (a *fakeAdapter) Update(_ context.Context, id string) (bool, error) {
	a.updated++
	a.updatedID = id
	return a.updateChange, a.updateErr
}
func (a *fakeAdapter) Delete(_ context.Context, id string) error {
	a.deleted++
	a.deletedID = id
	return a.deleteErr
}

func notFound() error {
	return &authentik.APIError{Op: "find", Kind: authentik.ErrNotFound, Detail: "no match"}
}

func TestSyncCreatesWhenNothingExists(t *testing.T) {
	obj := newFake("")
	ad := &fakeAdapter{findErr: notFound(), createID: "42"}

	out, err := Sync(context.Background(), SyncRequest{Object: obj, Adapter: ad})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !out.Created || out.RemoteID != "42" {
		t.Errorf("got %+v, want Created with RemoteID 42", out)
	}
	if ad.created != 1 {
		t.Errorf("Create called %d times, want 1", ad.created)
	}
}

// Silent adoption is how an operator quietly takes over an object a human
// maintains, so the default must refuse.
func TestSyncRefusesPreExistingObjectByDefault(t *testing.T) {
	obj := newFake("")
	ad := &fakeAdapter{findID: "99"}

	_, err := Sync(context.Background(), SyncRequest{Object: obj, Adapter: ad})
	if err == nil {
		t.Fatal("expected an adoption conflict")
	}
	if !IsAdoptionConflict(err) {
		t.Fatalf("expected an adoption conflict, got %v", err)
	}
	if ad.created != 0 || ad.updated != 0 {
		t.Error("a refused adoption must not create or modify anything")
	}
	if reason, requeue := ResultFor(err); reason != authentikv1alpha1.ReasonAdoptionConflict || requeue {
		t.Errorf("ResultFor = (%s, %v), want (AdoptionConflict, false)", reason, requeue)
	}
}

func TestSyncAdoptsWhenPolicyAllows(t *testing.T) {
	obj := newFake("")
	obj.adoption = authentikv1alpha1.AdoptionPolicyAdoptExisting
	ad := &fakeAdapter{findID: "99", updateChange: true}

	out, err := Sync(context.Background(), SyncRequest{Object: obj, Adapter: ad})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !out.Adopted || out.RemoteID != "99" {
		t.Errorf("got %+v, want Adopted with RemoteID 99", out)
	}
	if ad.updatedID != "99" {
		t.Errorf("adopted object updated with id %q, want 99", ad.updatedID)
	}
}

// A crash between creating the remote object and writing status leaves a real
// object with no recorded ID. Re-finding it by name is recovery, not adoption.
func TestSyncReclaimingOwnObjectIsNotAdoption(t *testing.T) {
	obj := newFake("99")
	ad := &fakeAdapter{existsResult: false, findID: "99"}

	out, err := Sync(context.Background(), SyncRequest{Object: obj, Adapter: ad})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if out.Adopted {
		t.Error("reclaiming an object we already recorded must not count as adoption")
	}
	if out.RemoteID != "99" {
		t.Errorf("RemoteID = %q, want 99", out.RemoteID)
	}
}

func TestSyncCorrectsDriftOnKnownObject(t *testing.T) {
	obj := newFake("42")
	ad := &fakeAdapter{existsResult: true, updateChange: true}

	out, err := Sync(context.Background(), SyncRequest{Object: obj, Adapter: ad})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !out.Updated {
		t.Error("expected drift to be corrected")
	}
	if ad.created != 0 {
		t.Error("an existing object must not be recreated")
	}
}

// The recorded ID is preferred over the name, so renaming on either side does
// not produce a duplicate.
func TestSyncPrefersRecordedIDOverName(t *testing.T) {
	obj := newFake("42")
	ad := &fakeAdapter{existsResult: true, findID: "should-not-be-used"}

	out, err := Sync(context.Background(), SyncRequest{Object: obj, Adapter: ad})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if out.RemoteID != "42" {
		t.Errorf("RemoteID = %q, want the recorded 42", out.RemoteID)
	}
}

func TestSyncRecreatesObjectDeletedRemotely(t *testing.T) {
	obj := newFake("42")
	ad := &fakeAdapter{existsResult: false, findErr: notFound(), createID: "77"}

	out, err := Sync(context.Background(), SyncRequest{Object: obj, Adapter: ad})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !out.Created || out.RemoteID != "77" {
		t.Errorf("got %+v, want a recreated object with RemoteID 77", out)
	}
}

func TestSyncSkipsObjectsBeingDeleted(t *testing.T) {
	obj := newFake("42")
	now := metav1.Now()
	obj.DeletionTimestamp = &now
	ad := &fakeAdapter{existsResult: true}

	if _, err := Sync(context.Background(), SyncRequest{Object: obj, Adapter: ad}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if ad.created+ad.updated != 0 {
		t.Error("an object under deletion must not be created or updated")
	}
}

func TestFinalizeDeletesRemoteObject(t *testing.T) {
	obj := newFake("42")
	ad := &fakeAdapter{}

	done, err := Finalize(context.Background(), SyncRequest{Object: obj, Adapter: ad})
	if err != nil || !done {
		t.Fatalf("Finalize = (%v, %v), want (true, nil)", done, err)
	}
	if ad.deletedID != "42" {
		t.Errorf("deleted id %q, want 42", ad.deletedID)
	}
}

func TestFinalizeOrphanLeavesRemoteObject(t *testing.T) {
	obj := newFake("42")
	obj.deletion = authentikv1alpha1.DeletionPolicyOrphan
	ad := &fakeAdapter{}

	done, err := Finalize(context.Background(), SyncRequest{Object: obj, Adapter: ad})
	if err != nil || !done {
		t.Fatalf("Finalize = (%v, %v), want (true, nil)", done, err)
	}
	if ad.deleted != 0 {
		t.Error("Orphan must not delete the authentik object")
	}
}

// Treating an already-deleted object as an error would wedge the resource and,
// with it, deletion of its whole namespace.
func TestFinalizeTreatsMissingRemoteAsDone(t *testing.T) {
	obj := newFake("42")
	ad := &fakeAdapter{deleteErr: notFound()}

	done, err := Finalize(context.Background(), SyncRequest{Object: obj, Adapter: ad})
	if err != nil || !done {
		t.Fatalf("Finalize = (%v, %v), want (true, nil)", done, err)
	}
}

func TestFinalizeWithNothingCreatedIsDone(t *testing.T) {
	obj := newFake("")
	ad := &fakeAdapter{}

	done, err := Finalize(context.Background(), SyncRequest{Object: obj, Adapter: ad})
	if err != nil || !done {
		t.Fatalf("Finalize = (%v, %v), want (true, nil)", done, err)
	}
	if ad.deleted != 0 {
		t.Error("nothing was created, so nothing should be deleted")
	}
}

func TestFinalizePropagatesRealDeleteFailure(t *testing.T) {
	obj := newFake("42")
	boom := errors.New("authentik exploded")
	ad := &fakeAdapter{deleteErr: boom}

	done, err := Finalize(context.Background(), SyncRequest{Object: obj, Adapter: ad})
	if done {
		t.Error("a failed delete must not allow the finalizer to be removed")
	}
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the underlying failure", err)
	}
}

// "Not yet" must be retried; "never" must not spin.
func TestResultForDistinguishesRetryableFailures(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantReason string
		wantRetry  bool
	}{
		{"success", nil, authentikv1alpha1.ReasonSucceeded, false},
		{"missing reference", notFound(), authentikv1alpha1.ReasonReferenceNotFound, true},
		{"ambiguous reference", &authentik.APIError{Kind: authentik.ErrAmbiguous}, authentikv1alpha1.ReasonReferenceAmbiguous, false},
		{"rejected spec", &authentik.APIError{Kind: authentik.ErrValidation}, authentikv1alpha1.ReasonInvalidSpec, false},
		{"connection missing", ErrConnectionNotFound, authentikv1alpha1.ReasonReferenceNotFound, true},
		{"connection not ready", ErrConnectionNotReady, authentikv1alpha1.ReasonConnectionNotReady, true},
		{"unknown", errors.New("boom"), authentikv1alpha1.ReasonAPIError, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason, retry := ResultFor(tc.err)
			if reason != tc.wantReason || retry != tc.wantRetry {
				t.Errorf("ResultFor = (%s, %v), want (%s, %v)", reason, retry, tc.wantReason, tc.wantRetry)
			}
		})
	}
}

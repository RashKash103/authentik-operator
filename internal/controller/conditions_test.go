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
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
)

func TestMarkReadySetsBothConditions(t *testing.T) {
	var conds []metav1.Condition
	MarkReady(&conds, 7)

	if !IsReady(conds) {
		t.Fatalf("expected Ready to be true, got %+v", conds)
	}
	if !meta.IsStatusConditionTrue(conds, authentikv1alpha1.ConditionSynced) {
		t.Errorf("expected Synced to be true, got %+v", conds)
	}
	for _, c := range conds {
		if c.ObservedGeneration != 7 {
			t.Errorf("condition %s: observedGeneration = %d, want 7", c.Type, c.ObservedGeneration)
		}
	}
}

func TestMarkSyncFailedClearsReady(t *testing.T) {
	var conds []metav1.Condition
	MarkReady(&conds, 1)
	MarkSyncFailed(&conds, authentikv1alpha1.ReasonAPIError, "connection refused", 2)

	if IsReady(conds) {
		t.Error("expected Ready to be false after a sync failure")
	}
	if meta.IsStatusConditionTrue(conds, authentikv1alpha1.ConditionSynced) {
		t.Error("expected Synced to be false after a sync failure")
	}
}

// MarkNotReady means "reached authentik, but not in sync yet" - typically an
// unresolved reference. Conflating that with a sync failure would hide which of
// the two actually happened.
func TestMarkNotReadyLeavesSyncedAlone(t *testing.T) {
	var conds []metav1.Condition
	MarkReady(&conds, 1)
	MarkNotReady(&conds, authentikv1alpha1.ReasonReferenceNotFound, "flow \"default-authz\" not found", 2)

	if IsReady(conds) {
		t.Error("expected Ready to be false")
	}
	if !meta.IsStatusConditionTrue(conds, authentikv1alpha1.ConditionSynced) {
		t.Error("expected Synced to remain true when only the reference is missing")
	}
}

// An over-long message makes the whole status update fail, which would hide the
// very error being reported.
func TestTruncateMessageStaysWithinAPILimit(t *testing.T) {
	long := strings.Repeat("x", maxConditionMessage+500)
	got := truncateMessage(long)

	if len(got) > maxConditionMessage {
		t.Fatalf("message length %d exceeds limit %d", len(got), maxConditionMessage)
	}
	if !strings.HasSuffix(got, "(truncated)") {
		t.Error("expected a truncation marker on a shortened message")
	}
	if short := truncateMessage("fine"); short != "fine" {
		t.Errorf("short message was altered: %q", short)
	}
}

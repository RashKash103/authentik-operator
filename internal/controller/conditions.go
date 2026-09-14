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

// Package controller contains the reconcilers for the authentik operator.
package controller

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
)

// SetCondition records a condition on the given slice, preserving
// LastTransitionTime when the status has not actually changed.
//
// observedGeneration is stamped onto the condition so that a `kubectl wait
// --for=condition=Ready` cannot be satisfied by a stale status left over from a
// previous spec.
func SetCondition(conditions *[]metav1.Condition, condType string, status metav1.ConditionStatus, reason, message string, observedGeneration int64) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            truncateMessage(message),
		ObservedGeneration: observedGeneration,
	})
}

// MarkReady records that the resource matches its desired spec.
func MarkReady(conditions *[]metav1.Condition, generation int64) {
	SetCondition(conditions, authentikv1alpha1.ConditionReady, metav1.ConditionTrue,
		authentikv1alpha1.ReasonSucceeded, "Resource is in sync with authentik", generation)
	SetCondition(conditions, authentikv1alpha1.ConditionSynced, metav1.ConditionTrue,
		authentikv1alpha1.ReasonSucceeded, "Last reconcile reached the authentik API", generation)
}

// MarkNotReady records that the resource does not yet match its desired spec.
// Synced is left alone: reaching the API and being in sync are different
// questions, and conflating them hides which one actually failed.
func MarkNotReady(conditions *[]metav1.Condition, reason, message string, generation int64) {
	SetCondition(conditions, authentikv1alpha1.ConditionReady, metav1.ConditionFalse,
		reason, message, generation)
}

// MarkSyncFailed records that the last reconcile could not reach or use the
// authentik API, which implies the resource is not Ready either.
func MarkSyncFailed(conditions *[]metav1.Condition, reason, message string, generation int64) {
	SetCondition(conditions, authentikv1alpha1.ConditionSynced, metav1.ConditionFalse,
		reason, message, generation)
	SetCondition(conditions, authentikv1alpha1.ConditionReady, metav1.ConditionFalse,
		reason, message, generation)
}

// IsReady reports whether the Ready condition is currently true.
func IsReady(conditions []metav1.Condition) bool {
	return meta.IsStatusConditionTrue(conditions, authentikv1alpha1.ConditionReady)
}

// maxConditionMessage is the API server's limit for a condition message.
const maxConditionMessage = 32768

// truncateMessage keeps a condition message within the API server's limit.
// authentik validation errors can be long, and an over-long message makes the
// whole status update fail, which would hide the very error being reported.
func truncateMessage(msg string) string {
	if len(msg) <= maxConditionMessage {
		return msg
	}
	const ellipsis = " (truncated)"
	return msg[:maxConditionMessage-len(ellipsis)] + ellipsis
}

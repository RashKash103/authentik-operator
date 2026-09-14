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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// defaultProbeInterval is used when a connection does not set one.
const defaultProbeInterval = 5 * time.Minute

// probeResult carries everything the caller needs to write status and decide
// when to look again.
type probeResult struct {
	status   authentikv1alpha1.AuthentikConnectionStatus
	requeue  time.Duration
	probeErr error
}

// probeConnection contacts an authentik instance and produces the status to
// record for it.
//
// It never returns an error for an unreachable or unsupported instance: those
// are ordinary states of the world that belong in conditions, not in the
// reconciler's error return, which would only produce log noise and a hot
// retry loop. A genuine client construction failure is still reported.
func probeConnection(
	ctx context.Context,
	build func() (authentik.Client, error),
	settings authentikv1alpha1.ConnectionSettings,
	generation int64,
	previous authentikv1alpha1.AuthentikConnectionStatus,
) probeResult {
	status := *previous.DeepCopy()
	status.ObservedGeneration = generation
	now := metav1.Now()
	status.LastProbeTime = &now

	interval := defaultProbeInterval
	if settings.ProbeInterval != nil && settings.ProbeInterval.Duration > 0 {
		interval = settings.ProbeInterval.Duration
	}

	client, err := build()
	if err != nil {
		MarkSyncFailed(&status.Conditions, ConnectionProblemReason(err), err.Error(), generation)
		status.VersionSupported = nil
		// Credentials tend to be fixed by editing a Secret, which triggers its
		// own reconcile, so there is no value in retrying aggressively here.
		return probeResult{status: status, requeue: interval, probeErr: nil}
	}

	compat, err := client.Compatibility(ctx)
	if err != nil {
		reason := authentikv1alpha1.ReasonAPIError
		if errors.Is(err, authentik.ErrUnauthorized) {
			reason = authentikv1alpha1.ReasonConnectionNotReady
		}
		MarkSyncFailed(&status.Conditions, reason, "cannot reach authentik: "+err.Error(), generation)
		status.VersionSupported = nil
		return probeResult{status: status, requeue: retryInterval(interval), probeErr: nil}
	}

	status.AuthentikVersion = compat.Raw
	supported := compat.Supported()
	status.VersionSupported = &supported

	// Reaching the API is Synced; being a version we support is Ready. Keeping
	// them apart is what lets a dependent resource say "the instance answered,
	// but I refuse to drive this version" instead of a vague failure.
	SetCondition(&status.Conditions, authentikv1alpha1.ConditionSynced, metav1.ConditionTrue,
		authentikv1alpha1.ReasonSucceeded, "Reached the authentik API", generation)

	if !supported {
		MarkNotReady(&status.Conditions, authentikv1alpha1.ReasonUnsupportedVersion, compat.Message, generation)
		return probeResult{status: status, requeue: interval}
	}

	SetCondition(&status.Conditions, authentikv1alpha1.ConditionReady, metav1.ConditionTrue,
		authentikv1alpha1.ReasonSucceeded, compat.Message, generation)
	return probeResult{status: status, requeue: interval}
}

// retryInterval shortens the wait after a failed probe, so a recovering
// instance is noticed promptly, while still bounding the retry rate.
func retryInterval(normal time.Duration) time.Duration {
	const failFast = 30 * time.Second
	if normal < failFast {
		return normal
	}
	return failFast
}

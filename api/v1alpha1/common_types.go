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

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ConnectionKind selects which connection CRD a ConnectionReference points at.
// +kubebuilder:validation:Enum=AuthentikConnection;ClusterAuthentikConnection
type ConnectionKind string

const (
	// ConnectionKindNamespaced refers to a namespaced AuthentikConnection.
	ConnectionKindNamespaced ConnectionKind = "AuthentikConnection"
	// ConnectionKindCluster refers to a cluster-scoped ClusterAuthentikConnection.
	ConnectionKindCluster ConnectionKind = "ClusterAuthentikConnection"
)

// ConnectionReference points at the authentik instance a resource belongs to.
//
// A namespaced AuthentikConnection is always resolved in the referring
// resource's own namespace. Cross-namespace references are deliberately not
// supported: they would let anyone who can create a resource in one namespace
// borrow credentials from another. Use a ClusterAuthentikConnection when a
// connection genuinely needs to be shared cluster-wide.
type ConnectionReference struct {
	// Kind of connection object being referenced.
	// +kubebuilder:default=AuthentikConnection
	// +optional
	Kind ConnectionKind `json:"kind,omitempty"`

	// Name of the connection object.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// LocalSecretKeyReference selects one key of a Secret in the same namespace as
// the referring object.
type LocalSecretKeyReference struct {
	// Name of the Secret.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key within the Secret's data.
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// SecretKeyReference selects one key of a Secret in an explicitly named
// namespace. Used by cluster-scoped objects, which have no namespace of their
// own to default to.
type SecretKeyReference struct {
	// Name of the Secret.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace holding the Secret.
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`

	// Key within the Secret's data.
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// DeletionPolicy controls what happens to the authentik object when the
// Kubernetes resource that manages it is deleted.
// +kubebuilder:validation:Enum=Delete;Orphan
type DeletionPolicy string

const (
	// DeletionPolicyDelete removes the authentik object along with the resource.
	DeletionPolicyDelete DeletionPolicy = "Delete"
	// DeletionPolicyOrphan leaves the authentik object in place.
	DeletionPolicyOrphan DeletionPolicy = "Orphan"
)

// AdoptionPolicy controls what happens when an authentik object with the same
// name or slug already exists.
//
// authentik objects are keyed by name/slug rather than by Kubernetes UID, so
// collisions are routine rather than exceptional. The default refuses to take
// over an existing object, because silent adoption is how an operator quietly
// overwrites something a human is maintaining by hand.
// +kubebuilder:validation:Enum=FailOnConflict;AdoptExisting
type AdoptionPolicy string

const (
	// AdoptionPolicyFailOnConflict refuses to manage a pre-existing object.
	AdoptionPolicyFailOnConflict AdoptionPolicy = "FailOnConflict"
	// AdoptionPolicyAdoptExisting takes ownership of a pre-existing object.
	AdoptionPolicyAdoptExisting AdoptionPolicy = "AdoptExisting"
)

// Condition types reported by every authentik-backed resource.
const (
	// ConditionReady is true when the authentik object matches the desired spec.
	ConditionReady = "Ready"
	// ConditionSynced is true when the last reconcile reached the authentik API.
	ConditionSynced = "Synced"
)

// Condition reasons. Controllers draw from this fixed set rather than inventing
// free-form strings, so that the troubleshooting guide can enumerate them.
const (
	// ReasonSucceeded indicates the resource reconciled cleanly.
	ReasonSucceeded = "Succeeded"
	// ReasonReferenceNotFound indicates a referenced object does not exist yet.
	ReasonReferenceNotFound = "ReferenceNotFound"
	// ReasonReferenceAmbiguous indicates a reference matched more than one object.
	ReasonReferenceAmbiguous = "ReferenceAmbiguous"
	// ReasonConnectionNotReady indicates the referenced connection is unusable.
	ReasonConnectionNotReady = "ConnectionNotReady"
	// ReasonAdoptionConflict indicates a pre-existing object blocks creation.
	ReasonAdoptionConflict = "AdoptionConflict"
	// ReasonUnsupportedVersion indicates the authentik version is out of range.
	ReasonUnsupportedVersion = "UnsupportedVersion"
	// ReasonInvalidSpec indicates authentik rejected the spec as invalid.
	ReasonInvalidSpec = "InvalidSpec"
	// ReasonAPIError indicates a transient or unexpected API failure.
	ReasonAPIError = "APIError"
	// ReasonDeleting indicates the resource is being torn down.
	ReasonDeleting = "Deleting"
)

// ManagedResourceStatus is embedded in every authentik-backed resource status.
type ManagedResourceStatus struct {
	// Conditions describe the current state of the resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the .metadata.generation this status reflects.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// RemoteID is authentik's own identifier for the managed object: a numeric
	// primary key for providers and applications, a UUID elsewhere.
	//
	// Once set this is the authoritative handle for the object. Lookups prefer
	// it over the name, so that renaming the object on either side does not
	// cause the operator to lose track of it and create a duplicate.
	// +optional
	RemoteID string `json:"remoteID,omitempty"`

	// RemoteName is the name or slug last observed in authentik. Informational.
	// +optional
	RemoteName string `json:"remoteName,omitempty"`

	// Adopted records that this resource took over a pre-existing authentik
	// object rather than creating it.
	// +optional
	Adopted bool `json:"adopted,omitempty"`

	// LastSyncedTime is when the resource last reconciled successfully.
	// +optional
	LastSyncedTime *metav1.Time `json:"lastSyncedTime,omitempty"`
}

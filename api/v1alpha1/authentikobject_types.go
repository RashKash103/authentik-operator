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

// Flow, PropertyMapping and CertificateKeyPair give the objects that providers
// point at a presence of their own.
//
// A provider used to name a flow by slug at every reference site, which meant
// the same slug appeared in a dozen manifests and nothing tied them together.
// Now one Flow resource identifies one authentik flow, and every provider
// references that resource — so the slug is written once, and swapping which
// flow is meant is a single edit.
//
// Today each of these adopts an object that already exists in authentik, named
// by `existingSlug` or `existingName`. They are the natural home for creating
// those objects instead, which is why the reference sites do not hold the slug:
// adding creation fields here later changes nothing for anything referencing
// them.

// AdoptedObjectStatus is the status shared by the adopt-only kinds.
type AdoptedObjectStatus struct {
	// Conditions describe the current state of the resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the .metadata.generation this status reflects.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// RemoteID is authentik's UUID for the resolved object. References are
	// resolved through this, so Kubernetes stays the source of truth and a
	// rename inside authentik cannot silently repoint a provider.
	// +optional
	RemoteID string `json:"remoteID,omitempty"`

	// RemoteName is the name or slug last observed in authentik.
	// +optional
	RemoteName string `json:"remoteName,omitempty"`

	// LastSyncedTime is when the object was last resolved successfully.
	// +optional
	LastSyncedTime *metav1.Time `json:"lastSyncedTime,omitempty"`
}

// FlowSpec identifies an authentik flow.
type FlowSpec struct {
	// ConnectionRef selects the authentik instance holding the flow.
	ConnectionRef ConnectionReference `json:"connectionRef"`

	// ExistingSlug is the slug of a flow that already exists in authentik,
	// such as "default-provider-authorization-explicit-consent". The operator
	// resolves it and never modifies the flow.
	//
	// A UUID is also accepted and passed through unchanged.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	ExistingSlug string `json:"existingSlug"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=akflow
// +kubebuilder:printcolumn:name="Slug",type=string,JSONPath=`.spec.existingSlug`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Flow identifies an authentik flow that providers can reference.
type Flow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FlowSpec            `json:"spec,omitempty"`
	Status AdoptedObjectStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FlowList contains a list of Flow.
type FlowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Flow `json:"items"`
}

// PropertyMappingSpec identifies an authentik property mapping.
type PropertyMappingSpec struct {
	// ConnectionRef selects the authentik instance holding the mapping.
	ConnectionRef ConnectionReference `json:"connectionRef"`

	// ExistingName is the name of a property mapping that already exists in
	// authentik, such as "authentik default OAuth Mapping: OpenID 'email'".
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	ExistingName string `json:"existingName"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=akmapping
// +kubebuilder:printcolumn:name="Mapping",type=string,JSONPath=`.spec.existingName`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PropertyMapping identifies an authentik property mapping that providers can
// reference.
type PropertyMapping struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PropertyMappingSpec `json:"spec,omitempty"`
	Status AdoptedObjectStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PropertyMappingList contains a list of PropertyMapping.
type PropertyMappingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PropertyMapping `json:"items"`
}

// CertificateKeyPairSpec identifies an authentik certificate-key pair.
type CertificateKeyPairSpec struct {
	// ConnectionRef selects the authentik instance holding the key pair.
	ConnectionRef ConnectionReference `json:"connectionRef"`

	// ExistingName is the name of a certificate-key pair that already exists
	// in authentik, such as "authentik Self-signed Certificate".
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	ExistingName string `json:"existingName"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=akkeypair
// +kubebuilder:printcolumn:name="Key pair",type=string,JSONPath=`.spec.existingName`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// CertificateKeyPair identifies an authentik certificate-key pair that
// providers can reference.
type CertificateKeyPair struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CertificateKeyPairSpec `json:"spec,omitempty"`
	Status AdoptedObjectStatus    `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CertificateKeyPairList contains a list of CertificateKeyPair.
type CertificateKeyPairList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CertificateKeyPair `json:"items"`
}

func init() {
	registerTypes(&Flow{}, &FlowList{})
	registerTypes(&PropertyMapping{}, &PropertyMappingList{})
	registerTypes(&CertificateKeyPair{}, &CertificateKeyPairList{})
}

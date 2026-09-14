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

// ClusterAuthentikConnectionSpec defines a cluster-wide connection to an
// authentik instance.
//
// SECURITY: because this object is cluster-scoped it must name the namespace
// holding its credentials explicitly, which means it can reference a Secret
// anywhere in the cluster. Permission to create or edit one of these is
// therefore close to a cluster-admin privilege, and RBAC for it should be
// granted accordingly. See SECURITY.md.
type ClusterAuthentikConnectionSpec struct {
	ConnectionSettings `json:",inline"`

	// TokenSecretRef points at a Secret holding an authentik API token. The
	// namespace is required and is the only place the token is read from; the
	// namespace of a resource referring to this connection is never consulted.
	TokenSecretRef SecretKeyReference `json:"tokenSecretRef"`

	// CABundleSecretRef optionally points at a Secret holding a PEM CA bundle
	// used to verify the authentik server.
	// +optional
	CABundleSecretRef *SecretKeyReference `json:"caBundleSecretRef,omitempty"`

	// AllowedNamespaces optionally restricts which namespaces may reference this
	// connection. An empty list means every namespace may use it.
	//
	// Without this, any user who can create a resource in any namespace can
	// drive an authentik instance they were never granted access to.
	// +optional
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=clakconn
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.authentikVersion`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterAuthentikConnection describes how to reach one authentik instance,
// usable from any namespace.
type ClusterAuthentikConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterAuthentikConnectionSpec `json:"spec,omitempty"`
	Status AuthentikConnectionStatus      `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterAuthentikConnectionList contains a list of ClusterAuthentikConnection.
type ClusterAuthentikConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterAuthentikConnection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterAuthentikConnection{}, &ClusterAuthentikConnectionList{})
}

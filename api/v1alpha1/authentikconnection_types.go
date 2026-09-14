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

// ConnectionSettings holds the transport configuration shared by the namespaced
// and cluster-scoped connection kinds.
type ConnectionSettings struct {
	// URL is the base URL of the authentik instance, for example
	// https://authentik.example.com. Do not include the /api/v3 suffix.
	// +kubebuilder:validation:Pattern=`^https?://`
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`

	// InsecureSkipTLSVerify disables verification of the authentik server's TLS
	// certificate. Intended for local testing against a self-signed instance;
	// prefer caBundleSecretRef anywhere else.
	// +kubebuilder:default=false
	// +optional
	InsecureSkipTLSVerify bool `json:"insecureSkipTLSVerify,omitempty"`

	// ProbeInterval is how often the connection is re-checked for reachability.
	// +kubebuilder:default="5m"
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern=`^([0-9]+(s|m|h))+$`
	// +optional
	ProbeInterval *metav1.Duration `json:"probeInterval,omitempty"`
}

// AuthentikConnectionSpec defines a connection to an authentik instance, using
// credentials held in the same namespace.
type AuthentikConnectionSpec struct {
	ConnectionSettings `json:",inline"`

	// TokenSecretRef points at a Secret in this object's own namespace holding
	// an authentik API token.
	TokenSecretRef LocalSecretKeyReference `json:"tokenSecretRef"`

	// CABundleSecretRef optionally points at a Secret in this object's own
	// namespace holding a PEM CA bundle used to verify the authentik server.
	// +optional
	CABundleSecretRef *LocalSecretKeyReference `json:"caBundleSecretRef,omitempty"`
}

// AuthentikConnectionStatus reports reachability and version compatibility.
type AuthentikConnectionStatus struct {
	// Conditions describe the current state of the connection.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the .metadata.generation this status reflects.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// AuthentikVersion is the version reported by the instance, e.g. "2026.8.2".
	// +optional
	AuthentikVersion string `json:"authentikVersion,omitempty"`

	// VersionSupported reports whether AuthentikVersion falls within the range
	// this operator is tested against. When false, dependent resources refuse to
	// reconcile rather than failing obscurely deep inside an API call.
	// +optional
	VersionSupported *bool `json:"versionSupported,omitempty"`

	// LastProbeTime is when the instance was last contacted.
	// +optional
	LastProbeTime *metav1.Time `json:"lastProbeTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=akconn
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.authentikVersion`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AuthentikConnection describes how to reach one authentik instance, using an
// API token stored in the same namespace.
type AuthentikConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuthentikConnectionSpec   `json:"spec,omitempty"`
	Status AuthentikConnectionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AuthentikConnectionList contains a list of AuthentikConnection.
type AuthentikConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuthentikConnection `json:"items"`
}

func init() {
	registerTypes(&AuthentikConnection{}, &AuthentikConnectionList{})
}

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

// ServiceConnectionCommonSpec holds the fields every outpost service
// connection shares.
//
// A service connection is what authentik uses to deploy and manage an outpost
// on the caller's behalf. It is not a provider, so it carries none of the flow
// references providers need; only identity and lifecycle policy are shared.
type ServiceConnectionCommonSpec struct {
	// ConnectionRef selects the authentik instance this service connection
	// lives in.
	ConnectionRef ConnectionReference `json:"connectionRef"`

	// Name is the service connection's name in authentik. Defaults to the
	// resource name.
	// +optional
	Name string `json:"name,omitempty"`

	// AdoptionPolicy controls what happens when a service connection with this
	// name already exists in authentik.
	// +kubebuilder:default=FailOnConflict
	// +optional
	Adoption AdoptionPolicy `json:"adoptionPolicy,omitempty"`

	// DeletionPolicy controls what happens to the authentik service connection
	// when this resource is deleted.
	// +kubebuilder:default=Delete
	// +optional
	Deletion DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// ServiceConnectionStatus is the status shared by every service connection
// kind.
type ServiceConnectionStatus struct {
	ManagedResourceStatus `json:",inline"`

	// ServiceConnectionID is authentik's UUID for this service connection. It
	// is what an Outpost's serviceConnectionRef ultimately resolves to, so it
	// is surfaced separately from the generic RemoteID string.
	// +optional
	ServiceConnectionID string `json:"serviceConnectionID,omitempty"`
}

// KubernetesServiceConnectionSpec defines a Kubernetes service connection.
//
// Either authentik runs inside the cluster it should deploy outposts into, in
// which case local is set and its own service account is used, or it needs a
// kubeconfig for a remote cluster. Setting both is rejected, because it hides
// which of the two credentials is actually in use.
// +kubebuilder:validation:XValidation:rule="(has(self.local) && self.local) != has(self.kubeconfigSecretRef)",message="set exactly one of local: true or kubeconfigSecretRef"
type KubernetesServiceConnectionSpec struct {
	ServiceConnectionCommonSpec `json:",inline"`

	// Local makes authentik use the cluster it is itself running in, through
	// its own service account, instead of a kubeconfig.
	// +optional
	Local *bool `json:"local,omitempty"`

	// KubeconfigSecretRef reads the kubeconfig for a remote cluster from a
	// Secret in this resource's namespace.
	//
	// A kubeconfig is never accepted inline: it is a cluster credential, and a
	// spec field would store it in plain text in etcd and print it in
	// `kubectl get -o yaml`.
	// +optional
	KubeconfigSecretRef *LocalSecretKeyReference `json:"kubeconfigSecretRef,omitempty"`

	// VerifySSL verifies the certificate presented by the Kubernetes API
	// endpoint. Defaults to true in authentik.
	// +optional
	VerifySSL *bool `json:"verifySSL,omitempty"`
}

// KubernetesServiceConnectionStatus reports the connection's state in
// authentik.
type KubernetesServiceConnectionStatus struct {
	ServiceConnectionStatus `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=akk8ssc
// +kubebuilder:printcolumn:name="Local",type=boolean,JSONPath=`.spec.local`
// +kubebuilder:printcolumn:name="Connection",type=string,JSONPath=`.status.serviceConnectionID`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KubernetesServiceConnection manages a Kubernetes outpost service connection
// in authentik.
type KubernetesServiceConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubernetesServiceConnectionSpec   `json:"spec,omitempty"`
	Status KubernetesServiceConnectionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KubernetesServiceConnectionList contains a list of KubernetesServiceConnection.
type KubernetesServiceConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubernetesServiceConnection `json:"items"`
}

// ConnectionRef implements controller.ManagedObject.
func (c *KubernetesServiceConnection) ConnectionRef() ConnectionReference {
	return c.Spec.ConnectionRef
}

// DeletionPolicy implements controller.ManagedObject.
func (c *KubernetesServiceConnection) DeletionPolicy() DeletionPolicy { return c.Spec.Deletion }

// AdoptionPolicy implements controller.ManagedObject.
func (c *KubernetesServiceConnection) AdoptionPolicy() AdoptionPolicy { return c.Spec.Adoption }

// ManagedStatus implements controller.ManagedObject.
func (c *KubernetesServiceConnection) ManagedStatus() *ManagedResourceStatus {
	return &c.Status.ManagedResourceStatus
}

// ServiceConnectionName is the name the connection carries in authentik,
// defaulting to the resource name when the spec does not override it.
func (c *KubernetesServiceConnection) ServiceConnectionName() string {
	if c.Spec.Name != "" {
		return c.Spec.Name
	}
	return c.Name
}

// The validation rule below spells its empty check as size() rather than
// comparing against an empty string literal: gofmt rewrites a doubled single
// quote inside a comment into a typographic quote, which would silently
// corrupt the expression. This note sits outside the doc comment on purpose,
// because a doc comment becomes the published CRD description.

// DockerServiceConnectionSpec defines a Docker service connection.
//
// Either authentik talks to the Docker socket it is mounted with, in which
// case local is set, or it dials an explicit URL. Setting both is rejected,
// because it hides which of the two endpoints is actually in use.
// +kubebuilder:validation:XValidation:rule="(has(self.local) && self.local) != (has(self.url) && self.url.size() > 0)",message="set exactly one of local: true or url"
type DockerServiceConnectionSpec struct {
	ServiceConnectionCommonSpec `json:",inline"`

	// Local makes authentik use the Docker socket mounted into its own
	// container instead of dialling a URL.
	// +optional
	Local *bool `json:"local,omitempty"`

	// URL of the Docker daemon, either "unix:///var/run/docker.sock" for a
	// local socket or "https://hostname:2376" for a remote daemon.
	// +optional
	URL string `json:"url,omitempty"`

	// TLSVerification is the certificate key pair holding the CA used to verify
	// the Docker daemon's certificate.
	// +optional
	TLSVerification *CertificateKeyPairReference `json:"tlsVerification,omitempty"`

	// TLSAuthentication is the certificate key pair presented to the Docker daemon
	// as a client certificate.
	// +optional
	TLSAuthentication *CertificateKeyPairReference `json:"tlsAuthentication,omitempty"`
}

// DockerServiceConnectionStatus reports the connection's state in authentik.
type DockerServiceConnectionStatus struct {
	ServiceConnectionStatus `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=akdockersc
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="Connection",type=string,JSONPath=`.status.serviceConnectionID`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DockerServiceConnection manages a Docker outpost service connection in
// authentik.
type DockerServiceConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DockerServiceConnectionSpec   `json:"spec,omitempty"`
	Status DockerServiceConnectionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DockerServiceConnectionList contains a list of DockerServiceConnection.
type DockerServiceConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DockerServiceConnection `json:"items"`
}

// ConnectionRef implements controller.ManagedObject.
func (c *DockerServiceConnection) ConnectionRef() ConnectionReference {
	return c.Spec.ConnectionRef
}

// DeletionPolicy implements controller.ManagedObject.
func (c *DockerServiceConnection) DeletionPolicy() DeletionPolicy { return c.Spec.Deletion }

// AdoptionPolicy implements controller.ManagedObject.
func (c *DockerServiceConnection) AdoptionPolicy() AdoptionPolicy { return c.Spec.Adoption }

// ManagedStatus implements controller.ManagedObject.
func (c *DockerServiceConnection) ManagedStatus() *ManagedResourceStatus {
	return &c.Status.ManagedResourceStatus
}

// ServiceConnectionName is the name the connection carries in authentik,
// defaulting to the resource name when the spec does not override it.
func (c *DockerServiceConnection) ServiceConnectionName() string {
	if c.Spec.Name != "" {
		return c.Spec.Name
	}
	return c.Name
}

func init() {
	registerTypes(&KubernetesServiceConnection{}, &KubernetesServiceConnectionList{})
	registerTypes(&DockerServiceConnection{}, &DockerServiceConnectionList{})
}

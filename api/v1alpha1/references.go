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

// References to the resources that identify authentik objects.
//
// These were once bare slugs and names written at each reference site, so the
// same flow slug appeared in every provider that used it and nothing connected
// them. A reference now names a Flow, PropertyMapping or CertificateKeyPair
// resource, and that resource says which authentik object is meant.
//
// The indirection buys two things. The identity of a flow is written once, so
// pointing every provider at a different one is a single edit. And references
// resolve through the referenced resource's status.remoteID rather than by
// name lookup, which keeps Kubernetes the source of truth and means a rename
// inside authentik cannot silently repoint a provider at something else.

// FlowReference points at a Flow resource in the same namespace.
//
// Cross-namespace references are deliberately absent, matching every other
// reference in this API: they would let anyone who can create a provider in one
// namespace borrow configuration from another.
type FlowReference struct {
	// Name of the Flow resource.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
	// Namespace holding the resource. Defaults to the referring resource's own
	// namespace.
	//
	// Naming another namespace requires the operator to be started with
	// cross-namespace references enabled; otherwise the reference is refused
	// rather than quietly resolved somewhere else.
	// +kubebuilder:validation:MaxLength=63
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// PropertyMappingReference points at a PropertyMapping resource in the same
// namespace.
type PropertyMappingReference struct {
	// Name of the PropertyMapping resource.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
	// Namespace holding the resource. Defaults to the referring resource's own
	// namespace.
	//
	// Naming another namespace requires the operator to be started with
	// cross-namespace references enabled; otherwise the reference is refused
	// rather than quietly resolved somewhere else.
	// +kubebuilder:validation:MaxLength=63
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// CertificateKeyPairReference points at a CertificateKeyPair resource in the
// same namespace.
type CertificateKeyPairReference struct {
	// Name of the CertificateKeyPair resource.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
	// Namespace holding the resource. Defaults to the referring resource's own
	// namespace.
	//
	// Naming another namespace requires the operator to be started with
	// cross-namespace references enabled; otherwise the reference is refused
	// rather than quietly resolved somewhere else.
	// +kubebuilder:validation:MaxLength=63
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ServiceConnectionReference points at a service connection resource in the
// same namespace.
//
// +kubebuilder:validation:XValidation:rule="has(self.kubernetesServiceConnectionName) != has(self.dockerServiceConnectionName)",message="exactly one of kubernetesServiceConnectionName or dockerServiceConnectionName must be set"
type ServiceConnectionReference struct {
	// KubernetesServiceConnectionName names a KubernetesServiceConnection
	// resource.
	// +kubebuilder:validation:MaxLength=253
	// +optional
	KubernetesServiceConnectionName string `json:"kubernetesServiceConnectionName,omitempty"`

	// DockerServiceConnectionName names a DockerServiceConnection resource.
	// +kubebuilder:validation:MaxLength=253
	// +optional
	DockerServiceConnectionName string `json:"dockerServiceConnectionName,omitempty"`

	// Namespace holding the resource. Defaults to the referring resource's own
	// namespace.
	//
	// Naming another namespace requires the operator to be started with
	// cross-namespace references enabled; otherwise the reference is refused
	// rather than quietly resolved somewhere else.
	// +kubebuilder:validation:MaxLength=63
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// PropertyMappingNames flattens a list of references to resource names.
func PropertyMappingNames(refs []PropertyMappingReference) []string {
	out := make([]string, 0, len(refs))
	for i := range refs {
		if refs[i].Name != "" {
			out = append(out, refs[i].Name)
		}
	}
	return out
}

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

// ProxyMode selects how the outpost serves the application.
// +kubebuilder:validation:Enum=proxy;forward_single;forward_domain
type ProxyMode string

const (
	// ProxyModeProxy terminates traffic in the outpost and forwards it to an
	// upstream host.
	ProxyModeProxy ProxyMode = "proxy"
	// ProxyModeForwardSingle authorizes one application behind an existing
	// reverse proxy.
	ProxyModeForwardSingle ProxyMode = "forward_single"
	// ProxyModeForwardDomain authorizes every application on a domain behind
	// an existing reverse proxy.
	ProxyModeForwardDomain ProxyMode = "forward_domain"
)

// ProxyProviderSpec defines a proxy provider served by an authentik outpost.
//
// The forwarding modes have no upstream of their own: an existing reverse
// proxy already holds the connection and only asks authentik whether to allow
// it. Rejecting the combination here turns a silently ignored field into an
// error on apply.
// The emptiness test uses size() rather than a comparison against an empty
// string literal: a pair of adjacent single quotes inside a comment is
// rewritten by gofmt into a typographic quote, which silently corrupts the
// rule.
// +kubebuilder:validation:XValidation:rule="!((has(self.mode) && (self.mode == 'forward_single' || self.mode == 'forward_domain')) && has(self.internalHost) && size(self.internalHost) > 0)",message="internalHost is only valid when mode is proxy; forward_single and forward_domain have no upstream to forward to"
type ProxyProviderSpec struct {
	ProviderCommonSpec `json:",inline"`

	// ExternalHost is the URL the application is reached on by users, for
	// example "https://grafana.example.com". It is what the outpost matches
	// incoming requests against.
	// +kubebuilder:validation:MinLength=1
	ExternalHost string `json:"externalHost"`

	// InternalHost is the upstream the outpost forwards traffic to, for
	// example "http://grafana.monitoring.svc.cluster.local:3000". Valid only
	// in proxy mode.
	// +optional
	InternalHost string `json:"internalHost,omitempty"`

	// InternalHostSSLValidation verifies the upstream's TLS certificate.
	// Disable it only for an upstream using a self-signed certificate.
	// +optional
	InternalHostSSLValidation *bool `json:"internalHostSSLValidation,omitempty"`

	// Mode selects how the outpost serves the application: proxy terminates
	// traffic and forwards it upstream, while the forward modes authorize
	// requests for an existing reverse proxy.
	// +kubebuilder:default=proxy
	// +optional
	Mode ProxyMode `json:"mode,omitempty"`

	// Certificate is the name of the certificate key pair the outpost presents
	// for ExternalHost. Leave unset when TLS is terminated ahead of the
	// outpost.
	// +optional
	Certificate string `json:"certificate,omitempty"`

	// SkipPathRegex lists paths that bypass authentication, one regular
	// expression per line. Use it for health checks and public assets.
	//
	// Every request matching one of these expressions reaches the application
	// unauthenticated, so keep the expressions anchored and narrow.
	// +optional
	SkipPathRegex string `json:"skipPathRegex,omitempty"`

	// BasicAuthEnabled sends HTTP Basic credentials to the upstream, for
	// applications that cannot read authentication headers.
	// +optional
	BasicAuthEnabled *bool `json:"basicAuthEnabled,omitempty"`

	// BasicAuthUserAttribute is the user attribute holding the username sent
	// as HTTP Basic credentials.
	// +optional
	BasicAuthUserAttribute string `json:"basicAuthUserAttribute,omitempty"`

	// BasicAuthPasswordAttribute is the user attribute holding the password
	// sent as HTTP Basic credentials.
	// +optional
	BasicAuthPasswordAttribute string `json:"basicAuthPasswordAttribute,omitempty"`

	// InterceptHeaderAuth makes the outpost handle Authorization headers sent
	// by the client instead of passing them through to the application.
	// +optional
	InterceptHeaderAuth *bool `json:"interceptHeaderAuth,omitempty"`

	// CookieDomain is the domain the session cookie is issued for. Set it in
	// forward_domain mode so one session covers every application on the
	// domain.
	// +optional
	CookieDomain string `json:"cookieDomain,omitempty"`

	// AccessTokenValidity in authentik duration syntax, e.g. "hours=24".
	// +kubebuilder:validation:Pattern=`^(((microseconds|milliseconds|seconds|minutes|hours|days|weeks)=-?\d+);?)+$`
	// +optional
	AccessTokenValidity string `json:"accessTokenValidity,omitempty"`

	// RefreshTokenValidity in authentik duration syntax, e.g. "days=30".
	// +kubebuilder:validation:Pattern=`^(((microseconds|milliseconds|seconds|minutes|hours|days|weeks)=-?\d+);?)+$`
	// +optional
	RefreshTokenValidity string `json:"refreshTokenValidity,omitempty"`
}

// ProxyProviderStatus reports the provider's state in authentik.
type ProxyProviderStatus struct {
	ProviderStatus `json:",inline"`

	// Outposts names the outposts currently serving this provider. A proxy
	// provider that no outpost serves is unreachable, so an empty list is the
	// usual explanation for an application that cannot be opened.
	// +optional
	Outposts []string `json:"outposts,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=akproxy
// +kubebuilder:printcolumn:name="External Host",type=string,JSONPath=`.spec.externalHost`
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name="Provider",type=integer,JSONPath=`.status.providerID`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ProxyProvider manages a proxy provider in authentik.
type ProxyProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProxyProviderSpec   `json:"spec,omitempty"`
	Status ProxyProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProxyProviderList contains a list of ProxyProvider.
type ProxyProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProxyProvider `json:"items"`
}

// ConnectionRef implements controller.ManagedObject.
func (p *ProxyProvider) ConnectionRef() ConnectionReference { return p.Spec.ConnectionRef }

// DeletionPolicy implements controller.ManagedObject.
func (p *ProxyProvider) DeletionPolicy() DeletionPolicy { return p.Spec.Deletion }

// AdoptionPolicy implements controller.ManagedObject.
func (p *ProxyProvider) AdoptionPolicy() AdoptionPolicy { return p.Spec.Adoption }

// ManagedStatus implements controller.ManagedObject.
func (p *ProxyProvider) ManagedStatus() *ManagedResourceStatus {
	return &p.Status.ManagedResourceStatus
}

// ProviderName is the name the provider carries in authentik, defaulting to
// the resource name when the spec does not override it.
func (p *ProxyProvider) ProviderName() string {
	if p.Spec.Name != "" {
		return p.Spec.Name
	}
	return p.Name
}

func init() {
	registerTypes(&ProxyProvider{}, &ProxyProviderList{})
}

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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "goauthentik.io/api/v3"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// stubAuthentikClient satisfies authentik.Client for the reference resolution
// the adapters perform. Only the lookups the adapters actually call are
// implemented; the rest exist to satisfy the interface and would be a test bug
// if they were ever reached.
type stubAuthentikClient struct {
	authentik.Client

	baseURL string

	// certKeyPairs maps a certificate key pair name to the UUID it resolves
	// to. A name that is absent resolves to a not-found error.
	certKeyPairs map[string]string

	// serviceConnections maps a service connection name to its UUID.
	serviceConnections map[string]string

	// providers maps the name of a provider that already exists in authentik
	// to its primary key. Used by references that name one directly rather
	// than naming a resource this operator manages.
	providers map[string]int32

	// cluster scopes managed object names. Empty in most tests, which is the
	// single-operator case.
	cluster string
}

func (c *stubAuthentikClient) BaseURL() string { return c.baseURL }

// Cluster must be declared rather than left to the embedded interface: the
// embedded value is nil, so an inherited call panics instead of returning "".
func (c *stubAuthentikClient) Cluster() string { return c.cluster }

func (c *stubAuthentikClient) ResolveCertificateKeyPair(_ context.Context, name string) (string, error) {
	if uuid, ok := c.certKeyPairs[name]; ok {
		return uuid, nil
	}
	return "", authentik.NotFound("resolve certificate keypair", "certificate keypair", name)
}

func (c *stubAuthentikClient) ResolveProvider(_ context.Context, name string) (int32, error) {
	if pk, ok := c.providers[name]; ok {
		return pk, nil
	}
	return 0, authentik.NotFound("resolve provider", "provider", name)
}

func (c *stubAuthentikClient) ResolveServiceConnection(_ context.Context, name string) (string, error) {
	if uuid, ok := c.serviceConnections[name]; ok {
		return uuid, nil
	}
	return "", authentik.NotFound("resolve service connection", "service connection", name)
}

func TestKubernetesConnectionRequestUsesSpec(t *testing.T) {
	cases := []struct {
		name           string
		spec           authentikv1alpha1.KubernetesServiceConnectionSpec
		kubeconfig     map[string]any
		wantLocal      bool
		wantVerifySSL  *bool
		wantKubeconfig bool
	}{
		{
			// The common in-cluster case: authentik deploys into the cluster it
			// runs in, so there is no kubeconfig to send at all.
			name:      "local uses no kubeconfig",
			spec:      authentikv1alpha1.KubernetesServiceConnectionSpec{Local: ptr(true)},
			wantLocal: true,
		},
		{
			// A remote cluster: the kubeconfig the controller read out of the
			// Secret has to reach the request, or authentik has no credentials.
			name:           "remote cluster sends the kubeconfig",
			spec:           authentikv1alpha1.KubernetesServiceConnectionSpec{},
			kubeconfig:     map[string]any{"apiVersion": "v1"},
			wantLocal:      false,
			wantKubeconfig: true,
		},
		{
			// verifySSL is tri-state: unset must stay unset so authentik's own
			// default applies rather than this operator inventing one.
			name:          "verifySSL is forwarded when set",
			spec:          authentikv1alpha1.KubernetesServiceConnectionSpec{Local: ptr(true), VerifySSL: ptr(false)},
			wantLocal:     true,
			wantVerifySSL: ptr(false),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conn := &authentikv1alpha1.KubernetesServiceConnection{Spec: tc.spec}
			conn.Name = "cluster"

			adapter := &kubernetesServiceConnectionAdapter{client: &stubAuthentikClient{}, conn: conn, kubeconfig: tc.kubeconfig}
			req := adapter.buildRequest()

			if req.GetName() != "cluster" {
				t.Errorf("name = %q, want the resource name", req.GetName())
			}
			if req.GetLocal() != tc.wantLocal {
				t.Errorf("local = %v, want %v", req.GetLocal(), tc.wantLocal)
			}
			if got := req.Kubeconfig != nil; got != tc.wantKubeconfig {
				t.Errorf("kubeconfig present = %v, want %v", got, tc.wantKubeconfig)
			}
			if tc.wantVerifySSL == nil {
				if req.VerifySsl != nil {
					t.Error("verifySSL must stay unset so authentik's default applies")
				}
			} else if req.GetVerifySsl() != *tc.wantVerifySSL {
				t.Errorf("verifySSL = %v, want %v", req.GetVerifySsl(), *tc.wantVerifySSL)
			}
		})
	}
}

// spec.name overrides the resource name, which is how one authentik object can
// be managed from a resource that has to be called something else.
func TestServiceConnectionNameFallsBackToResourceName(t *testing.T) {
	k8s := &authentikv1alpha1.KubernetesServiceConnection{}
	k8s.Name = "resource-name"
	if got := k8s.ServiceConnectionName(); got != "resource-name" {
		t.Errorf("ServiceConnectionName() = %q, want the resource name", got)
	}
	k8s.Spec.Name = "authentik-name"
	if got := k8s.ServiceConnectionName(); got != "authentik-name" {
		t.Errorf("ServiceConnectionName() = %q, want the spec override", got)
	}

	docker := &authentikv1alpha1.DockerServiceConnection{}
	docker.Name = "resource-name"
	if got := docker.ServiceConnectionName(); got != "resource-name" {
		t.Errorf("ServiceConnectionName() = %q, want the resource name", got)
	}
}

func TestDockerConnectionRequestResolvesCertificates(t *testing.T) {
	client := &stubAuthentikClient{certKeyPairs: map[string]string{
		"docker-ca":   "docker-ca-uuid",
		"docker-cert": "docker-cert-uuid",
	}}

	cases := []struct {
		name     string
		spec     authentikv1alpha1.DockerServiceConnectionSpec
		wantURL  string
		wantCA   string
		wantCert string
		wantErr  bool
	}{
		{
			// authentik's serializer rejects a blank url even for a local
			// connection, so local has to supply the conventional socket path
			// rather than leaving the field empty.
			name:    "local defaults the socket url",
			spec:    authentikv1alpha1.DockerServiceConnectionSpec{Local: ptr(true)},
			wantURL: defaultLocalDockerURL,
		},
		{
			name:     "remote resolves both certificate key pairs by name",
			spec:     authentikv1alpha1.DockerServiceConnectionSpec{URL: "https://docker:2376", TLSVerification: &authentikv1alpha1.CertificateKeyPairReference{Name: "docker-ca"}, TLSAuthentication: &authentikv1alpha1.CertificateKeyPairReference{Name: "docker-cert"}},
			wantURL:  "https://docker:2376",
			wantCA:   "docker-ca-uuid",
			wantCert: "docker-cert-uuid",
		},
		{
			// An unresolvable certificate must abort the request rather than
			// silently producing a connection with no TLS material.
			name:    "missing certificate key pair fails",
			spec:    authentikv1alpha1.DockerServiceConnectionSpec{URL: "https://docker:2376", TLSVerification: &authentikv1alpha1.CertificateKeyPairReference{Name: "absent"}},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conn := &authentikv1alpha1.DockerServiceConnection{
				ObjectMeta: metav1.ObjectMeta{Name: "docker", Namespace: "default"},
				Spec:       tc.spec,
			}
			conn.Name = "docker"

			adapter := &dockerServiceConnectionAdapter{client: client, kube: referenceFixture(t, "default", "default-authz", "default-invalidation", "default-authn", "openid", "email", "profile", "claims", "upn", "nameid", "authn-context", "saml-mapping", "proxy-cert", "signing-cert", "verification-cert", "encryption-cert", "docker-ca", "docker-cert", "absent-ok", "acr", "email-nameid"), conn: conn}
			req, err := adapter.buildRequest(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error for an unresolvable certificate key pair")
				}
				if !authentik.IsNotFound(err) {
					t.Errorf("err = %v, want a not-found so the resource requeues", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildRequest: %v", err)
			}

			if req.Url != tc.wantURL {
				t.Errorf("url = %q, want %q", req.Url, tc.wantURL)
			}
			if req.GetTlsVerification() != tc.wantCA {
				t.Errorf("tlsVerification = %q, want %q", req.GetTlsVerification(), tc.wantCA)
			}
			if req.GetTlsAuthentication() != tc.wantCert {
				t.Errorf("tlsAuthentication = %q, want %q", req.GetTlsAuthentication(), tc.wantCert)
			}
		})
	}
}

// Removing a certificate from the spec has to detach it in authentik. Leaving
// the field out of the request would keep the old certificate in place, which
// looks like the change was applied when it was not.
func TestDockerConnectionRequestClearsRemovedCertificates(t *testing.T) {
	conn := &authentikv1alpha1.DockerServiceConnection{
		Spec: authentikv1alpha1.DockerServiceConnectionSpec{URL: "https://docker:2376"},
	}
	conn.Name = "docker"

	adapter := &dockerServiceConnectionAdapter{client: &stubAuthentikClient{}, kube: referenceFixture(t, "default", "default-authz", "default-invalidation", "default-authn", "openid", "email", "profile", "claims", "upn", "nameid", "authn-context", "saml-mapping", "proxy-cert", "signing-cert", "verification-cert", "encryption-cert", "docker-ca", "docker-cert", "absent-ok", "acr", "email-nameid"), conn: conn}
	req, err := adapter.buildRequest(context.Background())
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}

	if !req.TlsVerification.IsSet() || req.TlsVerification.Get() != nil {
		t.Error("tlsVerification must be sent as an explicit null so authentik detaches it")
	}
	if !req.TlsAuthentication.IsSet() || req.TlsAuthentication.Get() != nil {
		t.Error("tlsAuthentication must be sent as an explicit null so authentik detaches it")
	}
}

// Equivalence drives only the "drift corrected" Event, but it must not report
// drift for a value the operator did not change, or every reconcile emits one.
func TestServiceConnectionEquivalence(t *testing.T) {
	dockerA := &api.DockerServiceConnection{Name: "docker", Url: "unix:///run/docker.sock"}
	dockerB := &api.DockerServiceConnection{Name: "docker", Url: "unix:///run/docker.sock"}
	if !dockerConnectionEquivalent(dockerA, dockerB) {
		t.Error("identical docker connections must compare equal")
	}
	dockerB.Url = "https://docker:2376"
	if dockerConnectionEquivalent(dockerA, dockerB) {
		t.Error("a changed url must be reported as drift")
	}

	k8sA := &api.KubernetesServiceConnection{Name: "cluster", Local: ptr(true)}
	k8sB := &api.KubernetesServiceConnection{Name: "cluster", Local: ptr(true)}
	if !kubernetesConnectionEquivalent(k8sA, k8sB) {
		t.Error("identical kubernetes connections must compare equal")
	}

	// The kubeconfig is deliberately excluded from the comparison: it is a
	// credential, and the update is sent unconditionally anyway.
	k8sB.Kubeconfig = map[string]any{"apiVersion": "v1"}
	if !kubernetesConnectionEquivalent(k8sA, k8sB) {
		t.Error("the kubeconfig must not take part in the drift comparison")
	}
}

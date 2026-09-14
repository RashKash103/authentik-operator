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

	api "goauthentik.io/api/v3"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// serviceConnectionPageSize bounds a name lookup. Service connections are
// few, so one page is always enough to spot a duplicate name.
const serviceConnectionPageSize = 100

// defaultLocalDockerURL is sent when a Docker service connection is local.
//
// authentik's serializer requires a non-blank url even for a local connection,
// where it is then ignored in favour of the mounted socket. Sending the
// conventional socket path keeps `local: true` from being rejected outright
// while leaving the CRD free of a field the user must not have to think about.
const defaultLocalDockerURL = "unix:///var/run/docker.sock"

// kubernetesServiceConnectionAdapter implements RemoteAdapter for a Kubernetes
// service connection.
type kubernetesServiceConnectionAdapter struct {
	client authentik.Client
	conn   *authentikv1alpha1.KubernetesServiceConnection

	// kubeconfig is the parsed contents of the referenced Secret, or nil for a
	// local connection. It is a cluster credential: it is passed straight into
	// the request and never logged, compared or placed in status.
	kubeconfig map[string]any

	// observed holds the connection as last read from authentik.
	observed *api.KubernetesServiceConnection
}

func (a *kubernetesServiceConnectionAdapter) Kind() string {
	return "Kubernetes service connection"
}

func (a *kubernetesServiceConnectionAdapter) DesiredName() string {
	return a.conn.ServiceConnectionName()
}

func (a *kubernetesServiceConnectionAdapter) Exists(ctx context.Context, id string) (bool, error) {
	const op = "retrieve kubernetes service connection"

	found, resp, err := a.client.API().OutpostsAPI.
		OutpostsServiceConnectionsKubernetesRetrieve(ctx, id).Execute()
	if err != nil {
		mapped := authentik.MapResponseError(op, resp, err)
		if authentik.IsNotFound(mapped) {
			return false, nil
		}
		return false, mapped
	}
	a.observed = found
	return true, nil
}

func (a *kubernetesServiceConnectionAdapter) FindByName(ctx context.Context) (string, error) {
	const op = "list kubernetes service connections"
	name := a.DesiredName()

	list, resp, err := a.client.API().OutpostsAPI.
		OutpostsServiceConnectionsKubernetesList(ctx).
		Name(name).PageSize(serviceConnectionPageSize).Execute()
	if err != nil {
		return "", authentik.MapResponseError(op, resp, err)
	}
	if list == nil {
		return "", authentik.NotFound(op, "Kubernetes service connection", name)
	}

	// The name filter is not guaranteed to be exact, so narrow client-side.
	var matches []api.KubernetesServiceConnection
	for i := range list.Results {
		if list.Results[i].Name == name {
			matches = append(matches, list.Results[i])
		}
	}

	switch len(matches) {
	case 0:
		return "", authentik.NotFound(op, "Kubernetes service connection", name)
	case 1:
		a.observed = &matches[0]
		return matches[0].Pk, nil
	default:
		return "", authentik.Ambiguous(op, "Kubernetes service connection", name, len(matches))
	}
}

func (a *kubernetesServiceConnectionAdapter) Create(ctx context.Context) (string, error) {
	const op = "create kubernetes service connection"

	created, resp, err := a.client.API().OutpostsAPI.
		OutpostsServiceConnectionsKubernetesCreate(ctx).
		KubernetesServiceConnectionRequest(*a.buildRequest()).Execute()
	if err != nil {
		return "", authentik.MapResponseError(op, resp, err)
	}
	a.observed = created
	return created.Pk, nil
}

func (a *kubernetesServiceConnectionAdapter) Update(ctx context.Context, id string) (bool, error) {
	const op = "update kubernetes service connection"

	// A full update is used rather than a partial one because the request is
	// built from the complete spec every time; sending it whole keeps the
	// remote object converging on the declared state.
	updated, resp, err := a.client.API().OutpostsAPI.
		OutpostsServiceConnectionsKubernetesUpdate(ctx, id).
		KubernetesServiceConnectionRequest(*a.buildRequest()).Execute()
	if err != nil {
		return false, authentik.MapResponseError(op, resp, err)
	}

	changed := a.observed == nil || !kubernetesConnectionEquivalent(a.observed, updated)
	a.observed = updated
	return changed, nil
}

func (a *kubernetesServiceConnectionAdapter) Delete(ctx context.Context, id string) error {
	const op = "delete kubernetes service connection"

	resp, err := a.client.API().OutpostsAPI.
		OutpostsServiceConnectionsKubernetesDestroy(ctx, id).Execute()
	if err != nil {
		return authentik.MapResponseError(op, resp, err)
	}
	return nil
}

// buildRequest turns the spec into an authentik request.
func (a *kubernetesServiceConnectionAdapter) buildRequest() *api.KubernetesServiceConnectionRequest {
	spec := a.conn.Spec

	req := api.NewKubernetesServiceConnectionRequest(a.DesiredName())
	req.SetLocal(spec.Local != nil && *spec.Local)
	if spec.VerifySSL != nil {
		req.SetVerifySsl(*spec.VerifySSL)
	}
	if len(a.kubeconfig) > 0 {
		req.SetKubeconfig(a.kubeconfig)
	}
	return req
}

// kubernetesConnectionEquivalent reports whether two connection states are the
// same in the fields this operator manages.
//
// The kubeconfig is deliberately excluded. It is a cluster credential, and the
// only thing this comparison feeds is a "drift corrected" Event; the update
// itself is sent unconditionally, so a rotated kubeconfig still reaches
// authentik without ever being diffed in memory.
func kubernetesConnectionEquivalent(a, b *api.KubernetesServiceConnection) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name &&
		a.GetLocal() == b.GetLocal() &&
		a.GetVerifySsl() == b.GetVerifySsl()
}

// dockerServiceConnectionAdapter implements RemoteAdapter for a Docker service
// connection.
type dockerServiceConnectionAdapter struct {
	client authentik.Client
	conn   *authentikv1alpha1.DockerServiceConnection

	// observed holds the connection as last read from authentik.
	observed *api.DockerServiceConnection
}

func (a *dockerServiceConnectionAdapter) Kind() string { return "Docker service connection" }

func (a *dockerServiceConnectionAdapter) DesiredName() string {
	return a.conn.ServiceConnectionName()
}

func (a *dockerServiceConnectionAdapter) Exists(ctx context.Context, id string) (bool, error) {
	const op = "retrieve docker service connection"

	found, resp, err := a.client.API().OutpostsAPI.
		OutpostsServiceConnectionsDockerRetrieve(ctx, id).Execute()
	if err != nil {
		mapped := authentik.MapResponseError(op, resp, err)
		if authentik.IsNotFound(mapped) {
			return false, nil
		}
		return false, mapped
	}
	a.observed = found
	return true, nil
}

func (a *dockerServiceConnectionAdapter) FindByName(ctx context.Context) (string, error) {
	const op = "list docker service connections"
	name := a.DesiredName()

	list, resp, err := a.client.API().OutpostsAPI.
		OutpostsServiceConnectionsDockerList(ctx).
		Name(name).PageSize(serviceConnectionPageSize).Execute()
	if err != nil {
		return "", authentik.MapResponseError(op, resp, err)
	}
	if list == nil {
		return "", authentik.NotFound(op, "Docker service connection", name)
	}

	var matches []api.DockerServiceConnection
	for i := range list.Results {
		if list.Results[i].Name == name {
			matches = append(matches, list.Results[i])
		}
	}

	switch len(matches) {
	case 0:
		return "", authentik.NotFound(op, "Docker service connection", name)
	case 1:
		a.observed = &matches[0]
		return matches[0].Pk, nil
	default:
		return "", authentik.Ambiguous(op, "Docker service connection", name, len(matches))
	}
}

func (a *dockerServiceConnectionAdapter) Create(ctx context.Context) (string, error) {
	const op = "create docker service connection"

	req, err := a.buildRequest(ctx)
	if err != nil {
		return "", err
	}

	created, resp, err := a.client.API().OutpostsAPI.
		OutpostsServiceConnectionsDockerCreate(ctx).
		DockerServiceConnectionRequest(*req).Execute()
	if err != nil {
		return "", authentik.MapResponseError(op, resp, err)
	}
	a.observed = created
	return created.Pk, nil
}

func (a *dockerServiceConnectionAdapter) Update(ctx context.Context, id string) (bool, error) {
	const op = "update docker service connection"

	req, err := a.buildRequest(ctx)
	if err != nil {
		return false, err
	}

	updated, resp, err := a.client.API().OutpostsAPI.
		OutpostsServiceConnectionsDockerUpdate(ctx, id).
		DockerServiceConnectionRequest(*req).Execute()
	if err != nil {
		return false, authentik.MapResponseError(op, resp, err)
	}

	changed := a.observed == nil || !dockerConnectionEquivalent(a.observed, updated)
	a.observed = updated
	return changed, nil
}

func (a *dockerServiceConnectionAdapter) Delete(ctx context.Context, id string) error {
	const op = "delete docker service connection"

	resp, err := a.client.API().OutpostsAPI.
		OutpostsServiceConnectionsDockerDestroy(ctx, id).Execute()
	if err != nil {
		return authentik.MapResponseError(op, resp, err)
	}
	return nil
}

// buildRequest turns the spec into an authentik request, resolving the
// certificate key pairs by name to the UUIDs authentik expects.
func (a *dockerServiceConnectionAdapter) buildRequest(ctx context.Context) (*api.DockerServiceConnectionRequest, error) {
	spec := a.conn.Spec

	local := spec.Local != nil && *spec.Local
	url := spec.URL
	if local && url == "" {
		url = defaultLocalDockerURL
	}

	req := api.NewDockerServiceConnectionRequest(a.DesiredName(), url)
	req.SetLocal(local)

	// Both certificate references are sent on every request, explicitly
	// cleared when unset, so removing one from the spec actually detaches it
	// rather than silently leaving the old certificate in place.
	if spec.TLSVerification != "" {
		pair, err := a.client.ResolveCertificateKeyPair(ctx, spec.TLSVerification)
		if err != nil {
			return nil, err
		}
		req.SetTlsVerification(pair)
	} else {
		req.SetTlsVerificationNil()
	}
	if spec.TLSAuthentication != "" {
		pair, err := a.client.ResolveCertificateKeyPair(ctx, spec.TLSAuthentication)
		if err != nil {
			return nil, err
		}
		req.SetTlsAuthentication(pair)
	} else {
		req.SetTlsAuthenticationNil()
	}

	return req, nil
}

// dockerConnectionEquivalent reports whether two connection states are the
// same in the fields this operator manages. Fields authentik computes are
// ignored, so a server-side default never looks like drift.
func dockerConnectionEquivalent(a, b *api.DockerServiceConnection) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name &&
		a.GetLocal() == b.GetLocal() &&
		a.Url == b.Url &&
		a.GetTlsVerification() == b.GetTlsVerification() &&
		a.GetTlsAuthentication() == b.GetTlsAuthentication()
}

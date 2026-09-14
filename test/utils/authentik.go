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

package utils

import (
	"context"
	"testing"
	"time"

	api "goauthentik.io/api/v3"

	"rka.sh/authentik-operator/internal/authentik"
)

// NewAuthentikClient builds a client for mutating authentik directly.
//
// The end-to-end suite uses this to change or remove objects behind the
// operator's back, which is the only way to prove drift correction and
// recreation actually work against a real instance.
func NewAuthentikClient(t *testing.T, baseURL, token string) authentik.Client {
	t.Helper()

	c, err := authentik.New(authentik.Config{
		BaseURL: baseURL,
		Token:   token,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("building authentik client: %v", err)
	}
	return c
}

// SetOAuth2SubMode changes a provider's sub mode directly in authentik.
func SetOAuth2SubMode(t *testing.T, ctx context.Context, c authentik.Client, pk int32, mode string) {
	t.Helper()

	patch := api.NewPatchedOAuth2ProviderRequest()
	patch.SetSubMode(api.SubModeEnum(mode))

	_, resp, err := c.API().ProvidersAPI.ProvidersOauth2PartialUpdate(ctx, pk).
		PatchedOAuth2ProviderRequest(*patch).Execute()
	if err != nil {
		t.Fatalf("patching provider %d out of band: %v",
			pk, authentik.MapResponseError("patch provider", resp, err))
	}
}

// DeleteOAuth2Provider removes a provider directly in authentik.
func DeleteOAuth2Provider(t *testing.T, ctx context.Context, c authentik.Client, pk int32) {
	t.Helper()

	resp, err := c.API().ProvidersAPI.ProvidersOauth2Destroy(ctx, pk).Execute()
	if err != nil {
		t.Fatalf("deleting provider %d out of band: %v",
			pk, authentik.MapResponseError("delete provider", resp, err))
	}
}

// WaitForOAuth2SubMode polls authentik until a provider reports the wanted sub
// mode, reporting what it actually saw on timeout.
func WaitForOAuth2SubMode(
	t *testing.T, ctx context.Context, c authentik.Client, pk int32, want string, timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		provider, _, err := c.API().ProvidersAPI.ProvidersOauth2Retrieve(ctx, pk).Execute()
		if err == nil {
			last = string(provider.GetSubMode())
			if last == want {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatalf("timed out after %s waiting for provider %d to report subMode %q; last saw %q",
		timeout, pk, want, last)
}

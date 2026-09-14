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
	"testing"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
)

func TestRotationRequested(t *testing.T) {
	cases := []struct {
		name          string
		annotation    string
		annotationSet bool
		observed      string
		want          bool
	}{
		{
			name: "no annotation means no rotation",
			want: false,
		},
		{
			name:          "an empty annotation is ignored",
			annotation:    "",
			annotationSet: true,
			want:          false,
		},
		{
			name:          "a new value triggers a rotation",
			annotation:    "2026-09-14T12:00:00Z",
			annotationSet: true,
			want:          true,
		},
		{
			// Without this, every reconcile would mint a new secret and break
			// workloads that had already loaded the previous one.
			name:          "a value already acted on does not rotate again",
			annotation:    "2026-09-14T12:00:00Z",
			annotationSet: true,
			observed:      "2026-09-14T12:00:00Z",
			want:          false,
		},
		{
			name:          "changing the value rotates again",
			annotation:    "2026-09-14T13:00:00Z",
			annotationSet: true,
			observed:      "2026-09-14T12:00:00Z",
			want:          true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := &authentikv1alpha1.OAuth2Provider{}
			if tc.annotationSet {
				provider.Annotations = map[string]string{
					authentikv1alpha1.RotateCredentialsAnnotation: tc.annotation,
				}
			}
			provider.Status.ObservedRotationToken = tc.observed

			if got := rotationRequested(provider); got != tc.want {
				t.Errorf("rotationRequested = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGenerateClientSecret(t *testing.T) {
	first, err := generateClientSecret()
	if err != nil {
		t.Fatalf("generateClientSecret: %v", err)
	}
	second, err := generateClientSecret()
	if err != nil {
		t.Fatalf("generateClientSecret: %v", err)
	}

	if first == second {
		t.Error("two generated secrets were identical")
	}
	// 48 random bytes in unpadded base64url. Short secrets are the kind of
	// thing that quietly weakens a confidential client.
	if len(first) < 60 {
		t.Errorf("generated secret is only %d characters, want at least 60", len(first))
	}
	for _, r := range first {
		isURLSafe := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_'
		if !isURLSafe {
			t.Errorf("generated secret contains %q, which is not URL-safe", r)
			break
		}
	}
}

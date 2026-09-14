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
	"crypto/sha256"

	corev1 "k8s.io/api/core/v1"
)

// HashSecretData replaces every value in a cached Secret with a digest of its
// contents.
//
// The operator watches Secrets so a rotated token takes effect without a
// restart. A watch means an informer, and an informer means every Secret in
// every watched namespace is held in memory — so by default the process would
// hold the plaintext of the entire cluster's Secrets, not just the handful its
// connections reference. Any read primitive against the process (a heap dump, a
// core file, a debug sidecar) would then yield all of them.
//
// Hashing in the cache transform keeps what the watch actually needs — the
// ability to notice that a value changed — while holding none of the values.
// Reads go straight to the API server instead (see the client cache options in
// main), so callers still get real data.
//
// Keys are preserved: a caller checking whether a key exists, and the predicate
// deciding whether anything changed, both work on the shape alone.
func HashSecretData(obj any) (any, error) {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return obj, nil
	}

	if len(secret.Data) == 0 && len(secret.StringData) == 0 {
		return secret, nil
	}

	// Copy rather than mutate: the object handed to a transform may be shared.
	out := secret.DeepCopy()
	for key, value := range out.Data {
		digest := sha256.Sum256(value)
		out.Data[key] = digest[:]
	}
	// StringData is write-only on the API and should never appear on a read,
	// but clear it rather than assume.
	out.StringData = nil

	return out, nil
}

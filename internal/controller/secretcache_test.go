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
	"bytes"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func secretWith(data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "token", Namespace: "apps"},
		Data:       data,
	}
}

func TestHashSecretDataRemovesPlaintext(t *testing.T) {
	plaintext := []byte("ak-super-secret-token")
	in := secretWith(map[string][]byte{"token": plaintext, "ca.crt": []byte("PEM")})

	out, err := HashSecretData(in)
	if err != nil {
		t.Fatalf("HashSecretData: %v", err)
	}
	hashed, ok := out.(*corev1.Secret)
	if !ok {
		t.Fatalf("expected a *corev1.Secret, got %T", out)
	}

	for key, value := range hashed.Data {
		if bytes.Contains(value, plaintext) || bytes.Equal(value, []byte("PEM")) {
			t.Errorf("key %q still holds its plaintext in the cache", key)
		}
		if len(value) != 32 {
			t.Errorf("key %q is %d bytes, want a 32-byte digest", key, len(value))
		}
	}

	// Keys must survive: callers check for a key's presence, and the change
	// predicate compares the shape.
	for _, key := range []string{"token", "ca.crt"} {
		if _, present := hashed.Data[key]; !present {
			t.Errorf("key %q was dropped from the cached Secret", key)
		}
	}

	// The transform must not mutate what it was handed; informers share it.
	if !bytes.Equal(in.Data["token"], plaintext) {
		t.Error("the input Secret was mutated in place")
	}
}

func TestHashSecretDataPassesOtherObjectsThrough(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "x"}}
	out, err := HashSecretData(pod)
	if err != nil {
		t.Fatalf("HashSecretData: %v", err)
	}
	if out != any(pod) {
		t.Error("a non-Secret object should be returned unchanged")
	}
}

// The whole point of hashing rather than stripping: the watch still has to
// notice a rotated token, and it decides that by comparing cached values.
func TestSecretChangePredicateStillWorksOnHashedData(t *testing.T) {
	hash := func(s *corev1.Secret) *corev1.Secret {
		out, err := HashSecretData(s)
		if err != nil {
			t.Fatalf("HashSecretData: %v", err)
		}
		return out.(*corev1.Secret)
	}

	oldSecret := hash(secretWith(map[string][]byte{"token": []byte("original")}))
	rotated := hash(secretWith(map[string][]byte{"token": []byte("rotated")}))
	unchanged := hash(secretWith(map[string][]byte{"token": []byte("original")}))

	pred := secretDataChanged()

	if !pred.Update(event.UpdateEvent{ObjectOld: oldSecret, ObjectNew: rotated}) {
		t.Error("a rotated token must still produce an event")
	}
	if pred.Update(event.UpdateEvent{ObjectOld: oldSecret, ObjectNew: unchanged}) {
		t.Error("an unchanged Secret must not produce an event")
	}
}

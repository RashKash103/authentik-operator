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
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SecretOwnershipError reports a refusal to write into a Secret this resource
// does not own.
type SecretOwnershipError struct {
	Secret types.NamespacedName
	Owner  string
}

func (e *SecretOwnershipError) Error() string {
	return fmt.Sprintf(
		"refusing to write Secret %s: it already exists and is not owned by %s. "+
			"Choose a name the operator can create, or delete the existing Secret if it is no longer needed",
		e.Secret, e.Owner)
}

// IsSecretOwnershipError reports whether err is a refusal to take over a Secret.
func IsSecretOwnershipError(err error) bool {
	var target *SecretOwnershipError
	return errors.As(err, &target)
}

// assertSecretWritable refuses to touch a Secret the given owner does not
// already control.
//
// Without this, the target Secret's name is attacker-chosen: anyone able to
// create a provider or outpost in a namespace could point writeCredentialsTo at
// an unrelated Secret and have the operator overwrite its keys, then stamp an
// owner reference on it so deleting the custom resource garbage-collects the
// victim. That turns "may create a custom resource" into "may destroy any
// Secret in the namespace", which is a privilege the RBAC never granted.
//
// A Secret that does not exist yet is fine, and one already controlled by this
// owner is fine. Anything else is refused.
func assertSecretWritable(
	ctx context.Context,
	c client.Client,
	key types.NamespacedName,
	owner client.Object,
) error {
	var existing corev1.Secret
	if err := c.Get(ctx, key, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			// Nothing there; the operator will create it.
			return nil
		}
		return err
	}

	// Compare UIDs rather than names: a deleted and recreated owner of the same
	// name is a different object, and should not inherit the old one's Secret.
	if ref := metav1.GetControllerOf(&existing); ref != nil && ref.UID == owner.GetUID() {
		return nil
	}

	return &SecretOwnershipError{
		Secret: key,
		Owner:  fmt.Sprintf("%s %s", owner.GetObjectKind().GroupVersionKind().Kind, owner.GetName()),
	}
}

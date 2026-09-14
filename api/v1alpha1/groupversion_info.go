// Package v1alpha1 contains API Schema definitions for the authentik v1alpha1 API group.
// +kubebuilder:object:generate=true
// +groupName=authentik.k8s.rka.sh
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is the group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "authentik.k8s.rka.sh", Version: "v1alpha1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	//
	// This uses apimachinery's builder rather than controller-runtime's
	// scheme.Builder, which is deprecated precisely so that api packages keep
	// their dependencies down to the standard library and apimachinery.
	SchemeBuilder = runtime.NewSchemeBuilder(func(s *runtime.Scheme) error {
		metav1.AddToGroupVersion(s, GroupVersion)
		return nil
	})

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

// registerTypes adds object kinds to the scheme under GroupVersion. Each types
// file calls it from init with its own kind and list kind.
func registerTypes(objects ...runtime.Object) {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, objects...)
		return nil
	})
}

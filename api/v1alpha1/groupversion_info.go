package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlscheme "sigs.k8s.io/controller-runtime/pkg/scheme"
)

const (
	Group   = "sandbox.kce.ksyun.com"
	Version = "v1alpha1"
)

var (
	GroupVersion  = schema.GroupVersion{Group: Group, Version: Version}
	SchemeBuilder = &ctrlscheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(
		&SandboxTemplate{},
		&SandboxTemplateList{},
		&Sandbox{},
		&SandboxList{},
		&SandboxClaim{},
		&SandboxClaimList{},
	)
}

func Kind(kind string) schema.GroupKind {
	return schema.GroupKind{Group: Group, Kind: kind}
}

func Resource(resource string) schema.GroupResource {
	return schema.GroupResource{Group: Group, Resource: resource}
}

var _ runtime.Object = &SandboxTemplate{}
var _ runtime.Object = &SandboxTemplateList{}
var _ runtime.Object = &Sandbox{}
var _ runtime.Object = &SandboxList{}
var _ runtime.Object = &SandboxClaim{}
var _ runtime.Object = &SandboxClaimList{}

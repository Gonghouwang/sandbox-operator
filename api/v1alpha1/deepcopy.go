package v1alpha1

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/runtime"
)

func copyJSON[T any](in *T) *T {
	if in == nil {
		return nil
	}
	out := new(T)
	b, err := json.Marshal(in)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(b, out)
	return out
}

func (in *SandboxTemplate) DeepCopyObject() runtime.Object {
	return copyJSON(in)
}

func (in *SandboxTemplateList) DeepCopyObject() runtime.Object {
	return copyJSON(in)
}

func (in *Sandbox) DeepCopyObject() runtime.Object {
	return copyJSON(in)
}

func (in *SandboxList) DeepCopyObject() runtime.Object {
	return copyJSON(in)
}

func (in *SandboxClaim) DeepCopyObject() runtime.Object {
	return copyJSON(in)
}

func (in *SandboxClaimList) DeepCopyObject() runtime.Object {
	return copyJSON(in)
}

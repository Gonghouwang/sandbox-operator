package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=sbxc
type SandboxClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SandboxClaimSpec   `json:"spec,omitempty"`
	Status SandboxClaimStatus `json:"status,omitempty"`
}

type SandboxClaimSpec struct {
	OpenAPICredentialRef *OpenAPICredentialReference `json:"openapiCredentialRef,omitempty"`
	Replicas             int                         `json:"replicas,omitempty"`
	TemplateRef          TemplateReference           `json:"templateRef"`
	TimeoutSeconds       int                         `json:"timeoutSeconds,omitempty"`
	Env                  []EnvVar                    `json:"env,omitempty"`
	Ks3MountConfig       *MountConfig                `json:"ks3MountConfig,omitempty"`
	KpfsMountConfig      *MountConfig                `json:"kpfsMountConfig,omitempty"`
	DeletionPolicy       DeletionPolicy              `json:"deletionPolicy,omitempty"`
}

type SandboxClaimStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Phase              Phase              `json:"phase,omitempty"`
	Desired            int                `json:"desired,omitempty"`
	Created            int                `json:"created,omitempty"`
	Ready              int                `json:"ready,omitempty"`
	Failed             int                `json:"failed,omitempty"`
	Sandboxes          []ClaimedSandbox   `json:"sandboxes,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

type ClaimedSandbox struct {
	Name      string `json:"name"`
	SandboxID string `json:"sandboxID,omitempty"`
	Phase     Phase  `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
type SandboxClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SandboxClaim `json:"items"`
}

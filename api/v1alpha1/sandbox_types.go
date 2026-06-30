package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=sbx
type Sandbox struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SandboxSpec   `json:"spec,omitempty"`
	Status SandboxStatus `json:"status,omitempty"`
}

type SandboxSpec struct {
	Name                 string                      `json:"name,omitempty"`
	OpenAPICredentialRef *OpenAPICredentialReference `json:"openapiCredentialRef,omitempty"`
	ClaimRef             *ClaimReference             `json:"claimRef,omitempty"`
	TemplateRef          TemplateReference           `json:"templateRef"`
	TimeoutSeconds       int                         `json:"timeoutSeconds,omitempty"`
	Env                  []EnvVar                    `json:"env,omitempty"`
	Ks3MountConfig       *MountConfig                `json:"ks3MountConfig,omitempty"`
	KpfsMountConfig      *MountConfig                `json:"kpfsMountConfig,omitempty"`
	DeletionPolicy       DeletionPolicy              `json:"deletionPolicy,omitempty"`
}

type SandboxStatus struct {
	ObservedGeneration  int64                       `json:"observedGeneration,omitempty"`
	SandboxID           string                      `json:"sandboxID,omitempty"`
	ExternalUpdatedAt   *metav1.Time                `json:"externalUpdatedAt,omitempty"`
	Template            *SandboxTemplateSummary     `json:"template,omitempty"`
	Phase               Phase                       `json:"phase,omitempty"`
	RawStatus           string                      `json:"rawStatus,omitempty"`
	TimeoutSeconds      int                         `json:"timeoutSeconds,omitempty"`
	CreateTime          *metav1.Time                `json:"createTime,omitempty"`
	EndTime             *metav1.Time                `json:"endTime,omitempty"`
	Endpoint            string                      `json:"endpoint,omitempty"`
	Token               string                      `json:"token,omitempty"`
	CustomConfiguration *SandboxCustomConfiguration `json:"customConfiguration,omitempty"`
	CredentialDrift     *CredentialDriftSet         `json:"credentialDrift,omitempty"`
	Conditions          []metav1.Condition          `json:"conditions,omitempty"`
}

type SandboxTemplateSummary struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Category string `json:"category,omitempty"`
}

type SandboxCustomConfiguration struct {
	ImageURL string `json:"imageUrl,omitempty"`
	Port     int    `json:"port,omitempty"`
	Command  string `json:"command,omitempty"`
}

// +kubebuilder:object:root=true
type SandboxList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Sandbox `json:"items"`
}

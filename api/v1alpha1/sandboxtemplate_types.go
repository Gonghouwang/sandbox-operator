package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=stpl
type SandboxTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SandboxTemplateSpec   `json:"spec,omitempty"`
	Status SandboxTemplateStatus `json:"status,omitempty"`
}

type SandboxTemplateSpec struct {
	OpenAPICredentialRef *OpenAPICredentialReference `json:"openapiCredentialRef,omitempty"`
	Description          string                      `json:"description,omitempty"`
	Category             string                      `json:"category,omitempty"`
	Type                 string                      `json:"type,omitempty"`
	Image                *ImageSpec                  `json:"image,omitempty"`
	Command              string                      `json:"command,omitempty"`
	Ports                []int                       `json:"ports,omitempty"`
	Resources            *ResourceSpec               `json:"resources,omitempty"`
	Network              *NetworkSpec                `json:"network,omitempty"`
	Env                  []EnvVar                    `json:"env,omitempty"`
	Ks3MountConfig       *MountConfig                `json:"ks3MountConfig,omitempty"`
	KpfsMountConfig      *MountConfig                `json:"kpfsMountConfig,omitempty"`
	Klog                 *KlogSpec                   `json:"klog,omitempty"`
	Preheat              *PreheatSpec                `json:"preheat,omitempty"`
	InstanceQuota        int                         `json:"instanceQuota,omitempty"`
	DeletionPolicy       DeletionPolicy              `json:"deletionPolicy,omitempty"`
}

type SandboxTemplateStatus struct {
	ObservedGeneration int64               `json:"observedGeneration,omitempty"`
	TemplateID         string              `json:"templateID,omitempty"`
	Phase              Phase               `json:"phase,omitempty"`
	RawStatus          string              `json:"rawStatus,omitempty"`
	ExternalUpdatedAt  *metav1.Time        `json:"externalUpdatedAt,omitempty"`
	CanDelete          bool                `json:"canDelete,omitempty"`
	CredentialDrift    *CredentialDriftSet `json:"credentialDrift,omitempty"`
	Klog               *KlogStatus         `json:"klog,omitempty"`
	Quota              *QuotaStatus        `json:"quota,omitempty"`
	Preheat            *PreheatStatus      `json:"preheat,omitempty"`
	CreatedAt          *metav1.Time        `json:"createdAt,omitempty"`
	UpdatedAt          *metav1.Time        `json:"updatedAt,omitempty"`
	Conditions         []metav1.Condition  `json:"conditions,omitempty"`
}

type KlogStatus struct {
	ProjectName string `json:"projectName,omitempty"`
	PoolName    string `json:"poolName,omitempty"`
}

type QuotaStatus struct {
	InstanceQuota                int `json:"instanceQuota,omitempty"`
	RemainingInstanceQuota       int `json:"remainingInstanceQuota,omitempty"`
	RemainingSystemInstanceQuota int `json:"remainingSystemInstanceQuota,omitempty"`
}

type PreheatStatus struct {
	Enabled                 bool `json:"enabled,omitempty"`
	Number                  int  `json:"number,omitempty"`
	PreheatedInstanceNumber int  `json:"preheatedInstanceNumber,omitempty"`
}

// +kubebuilder:object:root=true
type SandboxTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SandboxTemplate `json:"items"`
}

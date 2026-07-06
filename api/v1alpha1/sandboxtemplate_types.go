package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=stpl
// +kubebuilder:printcolumn:name="TemplateID",type=string,JSONPath=".metadata.annotations.sandbox\\.kce\\.ksyun\\.com/template-id"
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="Access",type=string,JSONPath=".spec.access"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Target",type=integer,JSONPath=".status.preheat.number"
// +kubebuilder:printcolumn:name="Preheated",type=integer,JSONPath=".status.preheat.preheatedInstanceNumber"
// +kubebuilder:printcolumn:name="Updated",type=date,JSONPath=".status.externalUpdatedAt"
type SandboxTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SandboxTemplateSpec   `json:"spec,omitempty"`
	Status SandboxTemplateStatus `json:"status,omitempty"`
}

type SandboxTemplateSpec struct {
	OpenAPICredentialRef *OpenAPICredentialReference `json:"openapiCredentialRef,omitempty"`
	Description          string                      `json:"description,omitempty"`
	Type                 string                      `json:"type,omitempty"`
	Access               string                      `json:"access,omitempty"`
	Template             *RuntimeTemplate            `json:"template,omitempty"`
}

type SandboxTemplateStatus struct {
	Phase             Phase               `json:"phase,omitempty"`
	ExternalUpdatedAt *metav1.Time        `json:"externalUpdatedAt,omitempty"`
	CanDelete         bool                `json:"canDelete,omitempty"`
	CredentialDrift   *CredentialDriftSet `json:"credentialDrift,omitempty"`
	Klog              *KlogStatus         `json:"klog,omitempty"`
	Quota             *QuotaStatus        `json:"quota,omitempty"`
	Preheat           *PreheatStatus      `json:"preheat,omitempty"`
	CreatedAt         *metav1.Time        `json:"createdAt,omitempty"`
	UpdatedAt         *metav1.Time        `json:"updatedAt,omitempty"`
	Conditions        []metav1.Condition  `json:"conditions,omitempty"`
}

// KlogStatus is kept only to clear status.klog from existing CRs during upgrade.
// New sync results write klog settings under spec.template.spec.observability.
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
	PreheatedInstanceNumber int  `json:"preheatedInstanceNumber"`
}

// +kubebuilder:object:root=true
type SandboxTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SandboxTemplate `json:"items"`
}

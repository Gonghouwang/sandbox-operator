package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=sbx
// +kubebuilder:printcolumn:name="SandboxID",type=string,JSONPath=".metadata.annotations.sandbox\\.kce\\.ksyun\\.com/sandbox-id"
// +kubebuilder:printcolumn:name="TemplateID",type=string,JSONPath=".metadata.annotations.sandbox\\.kce\\.ksyun\\.com/template-id"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Timeout",type=integer,JSONPath=".status.timeoutSeconds"
// +kubebuilder:printcolumn:name="EndTime",type=string,JSONPath=".status.endTime"
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
	StorageCredentialRef *LocalObjectReference       `json:"storageCredentialRef,omitempty"`
	Ks3MountConfig       *MountConfig                `json:"ks3MountConfig,omitempty"`
	KpfsMountConfig      *MountConfig                `json:"kpfsMountConfig,omitempty"`
}

type SandboxStatus struct {
	ExternalUpdatedAt   *metav1.Time                `json:"externalUpdatedAt,omitempty"`
	Phase               Phase                       `json:"phase,omitempty"`
	TimeoutSeconds      int                         `json:"timeoutSeconds,omitempty"`
	CreateTime          *metav1.Time                `json:"createTime,omitempty"`
	EndTime             *metav1.Time                `json:"endTime,omitempty"`
	Endpoint            string                      `json:"endpoint,omitempty"`
	URLs                *SandboxURLs                `json:"urls,omitempty"`
	AccessURL           *SandboxURLs                `json:"accessUrl,omitempty"`
	SdnsURLs            map[string]string           `json:"sdnsUrls,omitempty"`
	Token               string                      `json:"token,omitempty"`
	ImageURL            string                      `json:"imageUrl,omitempty"`
	Port                int                         `json:"port,omitempty"`
	Command             string                      `json:"command,omitempty"`
	Env                 []EnvVar                    `json:"env,omitempty"`
	Ks3MountConfig      *MountConfig                `json:"ks3MountConfig,omitempty"`
	KpfsMountConfig     *MountConfig                `json:"kpfsMountConfig,omitempty"`
	CustomConfiguration *SandboxCustomConfiguration `json:"customConfiguration,omitempty"`
	CredentialDrift     *CredentialDriftSet         `json:"credentialDrift,omitempty"`
	Conditions          []metav1.Condition          `json:"conditions,omitempty"`
}

type SandboxURLs struct {
	CdpURL      string `json:"cdpUrl,omitempty"`
	NoVncURL    string `json:"noVncUrl,omitempty"`
	CodeURL     string `json:"codeUrl,omitempty"`
	AppURL      string `json:"appUrl,omitempty"`
	TerminalURL string `json:"terminalUrl,omitempty"`
	VscodeURL   string `json:"vscodeUrl,omitempty"`
}

// SandboxCustomConfiguration is kept only to clear old status.customConfiguration
// from existing CRs during upgrade. New sync results write fields directly.
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

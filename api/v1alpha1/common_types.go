package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type DeletionPolicy string

const (
	DeletionPolicyDelete DeletionPolicy = "Delete"
	DeletionPolicyRetain DeletionPolicy = "Retain"
)

type Phase string

const (
	PhasePending    Phase = "Pending"
	PhaseReady      Phase = "Ready"
	PhaseStarting   Phase = "Starting"
	PhaseRunning    Phase = "Running"
	PhasePaused     Phase = "Paused"
	PhaseResuming   Phase = "Resuming"
	PhaseDeleting   Phase = "Deleting"
	PhaseFailed     Phase = "Failed"
	PhaseUnhealthy  Phase = "Unhealthy"
	PhaseUnknown    Phase = "Unknown"
	PhaseSuccessful Phase = "Successful"
)

const (
	ConditionReady           = "Ready"
	ConditionSynced          = "Synced"
	ConditionBound           = "Bound"
	ConditionCredentialDrift = "CredentialDrift"
)

type LocalObjectReference struct {
	Name string `json:"name"`
}

type OpenAPICredentialReference struct {
	Name string `json:"name"`
}

type TemplateReference struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type ClaimReference struct {
	Name string `json:"name"`
}

type EnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ResourceSpec struct {
	CPU      int `json:"cpu,omitempty"`
	MemoryGB int `json:"memoryGB,omitempty"`
}

type ImageSpec struct {
	Source             string                `json:"source,omitempty"`
	ImageURL           string                `json:"imageUrl,omitempty"`
	ImageEndpoint      string                `json:"imageEndpoint,omitempty"`
	ImageNamespace     string                `json:"imageNamespace,omitempty"`
	ImageName          string                `json:"imageName,omitempty"`
	ImageTag           string                `json:"imageTag,omitempty"`
	RegistryInstanceID string                `json:"registryInstanceId,omitempty"`
	CredentialRef      *LocalObjectReference `json:"credentialRef,omitempty"`
}

type NetworkSpec struct {
	PublicNetworkEnable        bool     `json:"publicNetworkEnable,omitempty"`
	PrivateNetworkEnable       bool     `json:"privateNetworkEnable,omitempty"`
	SharedInternetAccessEnable bool     `json:"sharedInternetAccessEnable,omitempty"`
	VPC                        *VPCSpec `json:"vpc,omitempty"`
}

type VPCSpec struct {
	VPCID    string `json:"vpcId,omitempty"`
	SubnetID string `json:"subnetId,omitempty"`
}

type PreheatSpec struct {
	Enabled bool `json:"enabled,omitempty"`
	Number  int  `json:"number,omitempty"`
}

type KlogSpec struct {
	Enabled       bool                  `json:"enabled,omitempty"`
	CredentialRef *LocalObjectReference `json:"credentialRef,omitempty"`
}

type MountConfig struct {
	Enabled       bool                  `json:"enabled,omitempty"`
	CredentialRef *LocalObjectReference `json:"credentialRef,omitempty"`
	MountPoints   []MountPoint          `json:"mountPoints,omitempty"`
}

type MountPoint struct {
	BucketName     string `json:"bucketName,omitempty"`
	FileSystemName string `json:"fileSystemName,omitempty"`
	RemotePath     string `json:"remotePath,omitempty"`
	LocalMountPath string `json:"localMountPath,omitempty"`
	ReadOnly       bool   `json:"readOnly,omitempty"`
}

type CredentialDriftStatus struct {
	InSync            string       `json:"inSync,omitempty"`
	Source            string       `json:"source,omitempty"`
	AccessKeyIDMasked string       `json:"accessKeyIdMasked,omitempty"`
	ObservedAt        *metav1.Time `json:"observedAt,omitempty"`
	Reason            string       `json:"reason,omitempty"`
}

type CredentialDriftSet struct {
	Image *CredentialDriftStatus `json:"image,omitempty"`
	KS3   *CredentialDriftStatus `json:"ks3,omitempty"`
	KPFS  *CredentialDriftStatus `json:"kpfs,omitempty"`
	Klog  *CredentialDriftStatus `json:"klog,omitempty"`
}

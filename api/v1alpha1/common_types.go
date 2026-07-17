package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	ConditionDeleting        = "Deleting"
	ConditionCredentialDrift = "CredentialDrift"
)

type LocalObjectReference struct {
	Name string `json:"name"`
}

type RegistryCredentialReference struct {
	Name   string `json:"name"`
	Server string `json:"server,omitempty"`
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

type RuntimeTemplate struct {
	Spec RuntimeTemplateSpec `json:"spec,omitempty"`
}

type RuntimeTemplateSpec struct {
	Image                *TemplateImageSpec    `json:"image,omitempty"`
	KecConfig            *RuntimeKecConfig     `json:"kecConfig,omitempty"`
	Ports                []ContainerPortSpec   `json:"ports,omitempty"`
	StartCommand         string                `json:"startCommand,omitempty"`
	Env                  []TemplateEnvVar      `json:"env,omitempty"`
	StorageCredentialRef *LocalObjectReference `json:"storageCredentialRef,omitempty"`
	Ks3MountConfig       *MountConfig          `json:"ks3MountConfig,omitempty"`
	KpfsMountConfig      *MountConfig          `json:"kpfsMountConfig,omitempty"`
	NetworkConfig        *OpenAPINetworkConfig `json:"networkConfig,omitempty"`
	SkillConfig          *SkillConfig          `json:"skillConfig,omitempty"`
	Pool                 *TemplatePoolSpec     `json:"pool,omitempty"`
	Observability        *ObservabilitySpec    `json:"observability,omitempty"`
}

type TemplateImageSpec struct {
	Source                string                       `json:"source,omitempty"`
	Image                 string                       `json:"image,omitempty"`
	RegistryCredentialRef *RegistryCredentialReference `json:"registryCredentialRef,omitempty"`
}

type ContainerPortSpec struct {
	Name          string `json:"name,omitempty"`
	ContainerPort int    `json:"containerPort,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
}

type TemplateEnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}

type OpenAPINetworkConfig struct {
	EnablePublic  bool   `json:"enablePublic,omitempty"`
	EnablePrivate bool   `json:"enablePrivate,omitempty"`
	CIDRBlock     string `json:"cidrBlock,omitempty"`
	// SharedInternetAccessEnable is normalized to true for public-only networks
	// and false for private-only networks before calling the OpenAPI.
	SharedInternetAccessEnable bool   `json:"sharedInternetAccessEnable,omitempty"`
	UserVpcID                  string `json:"userVpcId,omitempty"`
	UserSgID                   string `json:"userSgId,omitempty"`
	UserSubnetID               string `json:"userSubnetId,omitempty"`
	AvailabilityZone           string `json:"availabilityZone,omitempty"`
}

type SkillConfig struct {
	Enable            bool     `json:"enable,omitempty"`
	SpaceIDs          []string `json:"spaceIds,omitempty"`
	EnablePublicSkill bool     `json:"enablePublicSkill,omitempty"`
}

type RuntimeKecConfig struct {
	CPU           string            `json:"cpu,omitempty"`
	Memory        resource.Quantity `json:"memory,omitempty"`
	InstanceSpecs []KecInstanceSpec `json:"instanceSpecs,omitempty"`
}

type KecInstanceSpec struct {
	InstanceType string          `json:"instanceType,omitempty"`
	SystemDisk   *SystemDiskSpec `json:"systemDisk,omitempty"`
	DataDisks    []DataDiskSpec  `json:"dataDisks,omitempty"`
}

type SystemDiskSpec struct {
	Type string            `json:"type,omitempty"`
	Size resource.Quantity `json:"size,omitempty"`
}

type DataDiskSpec struct {
	Name               string            `json:"name,omitempty"`
	Type               string            `json:"type,omitempty"`
	Size               resource.Quantity `json:"size,omitempty"`
	DeleteWithInstance bool              `json:"deleteWithInstance,omitempty"`
	Path               string            `json:"path,omitempty"`
	FsType             string            `json:"fsType,omitempty"`
}

type TemplatePoolSpec struct {
	TargetSize int `json:"targetSize,omitempty"`
}

type ObservabilitySpec struct {
	Logging *LoggingSpec `json:"logging,omitempty"`
}

type LoggingSpec struct {
	Enabled           bool                  `json:"enabled,omitempty"`
	ProjectName       string                `json:"projectName,omitempty"`
	Endpoint          string                `json:"endpoint,omitempty"`
	ContainerPoolName string                `json:"containerPoolName,omitempty"`
	HostPoolName      string                `json:"hostPoolName,omitempty"`
	Rules             []string              `json:"rules,omitempty"`
	CredentialRef     *LocalObjectReference `json:"credentialRef,omitempty"`
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
	Enabled     bool         `json:"enabled,omitempty"`
	MountPoints []MountPoint `json:"mountPoints,omitempty"`
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

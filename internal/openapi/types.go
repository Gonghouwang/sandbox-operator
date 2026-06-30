package openapi

import "time"

type Credential struct {
	AccessKeyID     string
	SecretAccessKey string
	AccountID       string
	Region          string
}

type Template struct {
	TemplateID                   string
	TemplateName                 string
	Description                  string
	TemplateCategory             string
	TemplateType                 string
	ImageConfig                  *ImageConfig
	Command                      string
	Ports                        []int
	CPU                          int
	Memory                       int
	Envs                         []Env
	NetworkConfig                *NetworkConfig
	PreheatConfig                *PreheatConfig
	InstanceQuota                int
	Status                       string
	CanDelete                    bool
	CreatedAt                    *time.Time
	UpdatedAt                    *time.Time
	KlogProjectName              string
	KlogPoolName                 string
	RemainingInstanceQuota       int
	RemainingSystemInstanceQuota int
	PreheatedInstanceNumber      int
	CredentialAccessKeyIDMasked  string
}

type Sandbox struct {
	SandboxID           string
	TemplateID          string
	TemplateType        string
	TemplateCategory    string
	Status              string
	Timeout             int
	CreateTime          *time.Time
	EndTime             *time.Time
	Endpoint            string
	CustomConfiguration *CustomConfiguration
}

type ImageConfig struct {
	Source             string
	ImageURL           string
	ImageEndpoint      string
	ImageNamespace     string
	ImageName          string
	ImageTag           string
	RegistryInstanceID string
}

type NetworkConfig struct {
	PublicNetworkEnable        bool
	PrivateNetworkEnable       bool
	SharedInternetAccessEnable bool
	VPCID                      string
	SubnetID                   string
}

type PreheatConfig struct {
	Enabled bool
	Number  int
}

type Env struct {
	Key   string
	Value string
}

type MountConfig struct {
	Enabled         bool
	AccessKey       string
	SecretAccessKey string
	MountPoints     []MountPoint
}

type MountPoint struct {
	BucketName     string
	FileSystemName string
	RemotePath     string
	LocalMountPath string
	ReadOnly       bool
}

type CustomConfiguration struct {
	ImageURL string
	Port     int
	Command  string
}

type CreateTemplateRequest struct {
	TemplateName     string
	Description      string
	TemplateCategory string
	TemplateType     string
	ImageConfig      *ImageConfig
	Command          string
	Ports            []int
	CPU              int
	Memory           int
	Envs             []Env
	NetworkConfig    *NetworkConfig
	PreheatConfig    *PreheatConfig
	InstanceQuota    int
	KS3MountConfig   *MountConfig
	KPFSMountConfig  *MountConfig
	KlogEnabled      bool
}

type CreateTemplateResponse struct {
	TemplateID      string
	KlogProjectName string
	KlogPoolName    string
}

type UpdateTemplateRequest struct {
	TemplateID string
	CreateTemplateRequest
}

type StartSandboxRequest struct {
	TemplateID      string
	Timeout         int
	Envs            []Env
	KS3MountConfig  *MountConfig
	KPFSMountConfig *MountConfig
}

type StartSandboxResponse struct {
	SandboxID  string
	Endpoint   string
	TemplateID string
	Token      string
	Timeout    int
}

type ListTemplatesRequest struct {
	PageNum  int
	PageSize int
}

type ListSandboxesRequest struct {
	PageNum  int
	PageSize int
}

type TemplateList struct {
	Items []Template
	Total int
}

type SandboxList struct {
	Items []Sandbox
	Total int
}

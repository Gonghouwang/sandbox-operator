package openapi

type Credential struct {
	AccessKeyID     string
	SecretAccessKey string
	AccountID       string
	Region          string
}

type Template struct {
	ID                           string            `json:"id"`
	TemplateID                   string            `json:"templateID,omitempty"`
	TemplateName                 string            `json:"name"`
	Description                  string            `json:"description"`
	TemplateCategory             string            `json:"access"`
	TemplateType                 string            `json:"type"`
	Image                        string            `json:"image"`
	ImageSource                  string            `json:"imageSource"`
	CredentialServer             string            `json:"credentialServer"`
	CredentialUsername           string            `json:"credentialUname"`
	CredentialPassword           string            `json:"credentialPwd"`
	Command                      string            `json:"startCmd"`
	Ports                        []int             `json:"ports"`
	CPU                          int               `json:"cpuCount"`
	Memory                       int               `json:"memoryMB"`
	DiskSizeMB                   int64             `json:"diskSizeMB,omitempty"`
	Envs                         map[string]string `json:"envs"`
	NetworkConfig                *NetworkConfig    `json:"networkConfig"`
	TargetPoolSize               int               `json:"targetPoolSize"`
	InstanceQuota                int               `json:"quota"`
	Status                       string            `json:"status"`
	CanDelete                    bool              `json:"canDelete"`
	CreatedAt                    string            `json:"createdAt"`
	UpdatedAt                    string            `json:"updatedAt"`
	KlogProjectName              string            `json:"-"`
	KlogPoolName                 string            `json:"-"`
	RemainingInstanceQuota       int               `json:"remainingInstanceQuota"`
	RemainingSystemInstanceQuota int               `json:"remainingSystemInstanceQuota"`
	PreheatedInstanceNumber      int               `json:"preheatedInstanceNumber"`
	KS3MountConfig               *MountConfig      `json:"ks3MountConfig"`
	KPFSMountConfig              *MountConfig      `json:"kpfsMountConfig"`
	KlogConfig                   *KlogConfig       `json:"klogConfig"`
	SkillConfig                  *SkillConfig      `json:"skillConfig,omitempty"`
	DataDisks                    []DataDisk        `json:"dataDisks,omitempty"`
	CredentialAccessKeyIDMasked  string            `json:"credentialAccessKeyIDMasked"`
}

func (t Template) Identifier() string {
	if t.ID != "" {
		return t.ID
	}
	return t.TemplateID
}

type Sandbox struct {
	SandboxID           string `json:"sandboxID"`
	TemplateID          string `json:"templateID"`
	TemplateType        string `json:"templateType"`
	TemplateCategory    string `json:"templateCategory"`
	Status              string `json:"state"`
	Timeout             int    `json:"timeout"`
	CreateTime          string `json:"startedAt"`
	EndTime             string `json:"endAt"`
	Endpoint            string `json:"domain"`
	URLs                *URLs  `json:"urls"`
	EnvdAccessToken     string `json:"envdAccessToken"`
	CustomConfiguration *CustomConfiguration
}

type NetworkConfig struct {
	PublicNetworkEnable        bool   `json:"enablePublic"`
	PrivateNetworkEnable       bool   `json:"enablePrivate"`
	SharedInternetAccessEnable bool   `json:"changeDefaultRoute"`
	VPCID                      string `json:"userVpcId"`
	SubnetID                   string `json:"userSubnetId"`
	SecurityID                 string `json:"userSgId"`
	CIDRBlock                  string `json:"cidrBlock"`
	AvailabilityZone           string `json:"availabilityZone,omitempty"`
}

type MountConfig struct {
	EnableKS3   bool             `json:"enableKs3,omitempty"`
	EnableKPFS  bool             `json:"enableKpfs,omitempty"`
	Credential  *MountCredential `json:"credentials,omitempty"`
	MountPoints []MountPoint     `json:"mountPoints,omitempty"`
}

type MountCredential struct {
	AccessKey       string `json:"accessKey,omitempty"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
}

type MountPoint struct {
	BucketName     string `json:"bucketName,omitempty"`
	BucketPath     string `json:"bucketPath,omitempty"`
	FileSystemName string `json:"fileSystemName,omitempty"`
	RemotePath     string `json:"remotePath,omitempty"`
	LocalMountPath string `json:"localMountPath,omitempty"`
	ReadOnly       bool   `json:"readOnly,omitempty"`
	Token          string `json:"token,omitempty"`
}

type CustomConfiguration struct {
	ImageURL string `json:"imageUrl"`
	Port     int    `json:"port"`
	Command  string `json:"startCmd"`
}

type URLs struct {
	CdpURL      string `json:"CdpUrl,omitempty"`
	NoVncURL    string `json:"NoVncUrl,omitempty"`
	Code        string `json:"CodeUrl,omitempty"`
	AppURL      string `json:"AppUrl,omitempty"`
	TerminalURL string `json:"TerminalUrl,omitempty"`
	VscodeURL   string `json:"VscodeUrl,omitempty"`
}

type KlogConfig struct {
	Enabled           bool     `json:"enable"`
	AccessKey         string   `json:"accessKey,omitempty"`
	SecretKey         string   `json:"secretKey,omitempty"`
	ProjectName       string   `json:"projectName,omitempty"`
	KlogEndpoint      string   `json:"klogEndpoint,omitempty"`
	PoolNameContainer string   `json:"poolNameContainer,omitempty"`
	PoolNameHost      string   `json:"poolNameHost,omitempty"`
	Rules             []string `json:"rules,omitempty"`
}

type SkillConfig struct {
	Enable            bool   `json:"enable,omitempty"`
	SpaceID           string `json:"spaceId,omitempty"`
	EnablePublicSkill bool   `json:"enablePublicSkill,omitempty"`
}

type DataDisk struct {
	Name               string `json:"name,omitempty"`
	Type               string `json:"type,omitempty"`
	SizeMB             int64  `json:"sizeMB,omitempty"`
	DeleteWithInstance bool   `json:"deleteWithInstance,omitempty"`
	Path               string `json:"path,omitempty"`
}

type CreateTemplateRequest struct {
	TemplateName          string            `json:"name,omitempty"`
	Alias                 string            `json:"alias,omitempty"`
	Description           string            `json:"description,omitempty"`
	TemplateCategory      string            `json:"access,omitempty"`
	IsPublic              *bool             `json:"isPublic,omitempty"`
	TemplateType          string            `json:"type,omitempty"`
	Image                 string            `json:"image,omitempty"`
	CredentialServer      string            `json:"credentialServer,omitempty"`
	CredentialUsername    string            `json:"credentialUname,omitempty"`
	CredentialPassword    string            `json:"credentialPwd,omitempty"`
	ImageSource           string            `json:"imageSource,omitempty"`
	Command               string            `json:"startCmd,omitempty"`
	Ports                 []int             `json:"ports,omitempty"`
	CPU                   int               `json:"cpuCount,omitempty"`
	Memory                int               `json:"memoryMB,omitempty"`
	DiskSizeMB            int64             `json:"diskSizeMB,omitempty"`
	Envs                  map[string]string `json:"envs,omitempty"`
	NetworkConfig         *NetworkConfig    `json:"networkConfig,omitempty"`
	TargetPoolSize        int               `json:"targetPoolSize,omitempty"`
	InstanceQuota         int               `json:"quota,omitempty"`
	WarmPoolImagePullOnly *bool             `json:"warmPoolImagePullOnly,omitempty"`
	KS3MountConfig        *MountConfig      `json:"ks3MountConfig,omitempty"`
	KPFSMountConfig       *MountConfig      `json:"kpfsMountConfig,omitempty"`
	KlogConfig            *KlogConfig       `json:"klogConfig,omitempty"`
	SkillConfig           *SkillConfig      `json:"skillConfig,omitempty"`
	DataDisks             []DataDisk        `json:"dataDisks,omitempty"`
}

type CreateTemplateResponse struct {
	TemplateID      string `json:"templateID"`
	KlogProjectName string `json:"klogProjectName"`
	KlogPoolName    string `json:"poolNameContainer"`
}

type UpdateTemplateRequest struct {
	TemplateID string `json:"templateID,omitempty"`
	CreateTemplateRequest
}

type StartSandboxRequest struct {
	TemplateID          string                 `json:"templateID"`
	Timeout             int                    `json:"timeout,omitempty"`
	AllowInternetAccess *bool                  `json:"allowInternetAccess,omitempty"`
	EnvVars             map[string]string      `json:"envVars,omitempty"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
}

type StartSandboxResponse struct {
	SandboxID          string `json:"sandboxID"`
	Endpoint           string `json:"domain"`
	TemplateID         string `json:"templateID"`
	Token              string `json:"envdAccessToken"`
	TrafficAccessToken string `json:"trafficAccessToken"`
	Timeout            int    `json:"timeout"`
}

type ListTemplatesRequest struct {
	PageNum  int `json:"page"`
	PageSize int `json:"pageSize"`
}

type ListSandboxesRequest struct {
	PageNum  int `json:"page"`
	PageSize int `json:"pageSize"`
}

type TemplateList struct {
	Items     []Template `json:"templates"`
	NextToken string     `json:"nextToken"`
	Total     int        `json:"totalCount"`
}

type SandboxList struct {
	Items     []Sandbox `json:"sandboxes"`
	NextToken string    `json:"nextToken"`
	Total     int       `json:"totalCount"`
}

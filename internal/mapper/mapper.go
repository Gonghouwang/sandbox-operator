package mapper

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sandboxv1 "sandbox-operator/api/v1alpha1"
	"sandbox-operator/internal/credentials"
	"sandbox-operator/internal/openapi"
)

func OpenAPICredential(in *credentials.OpenAPICredential) openapi.Credential {
	if in == nil {
		return openapi.Credential{}
	}
	return openapi.Credential{
		AccessKeyID:     in.AccessKeyID,
		SecretAccessKey: in.SecretAccessKey,
		AccountID:       in.AccountID,
		Region:          in.Region,
	}
}

func TemplateCreateRequest(in *sandboxv1.SandboxTemplate, runtime RuntimeCredentials) openapi.CreateTemplateRequest {
	spec := in.Spec
	return openapi.CreateTemplateRequest{
		TemplateName:     in.Name,
		Description:      spec.Description,
		TemplateCategory: spec.Category,
		TemplateType:     spec.Type,
		ImageConfig:      imageToOpenAPI(spec.Image),
		Command:          spec.Command,
		Ports:            append([]int(nil), spec.Ports...),
		CPU:              resourceCPU(spec.Resources),
		Memory:           resourceMemory(spec.Resources),
		Envs:             envsToOpenAPI(spec.Env),
		NetworkConfig:    networkToOpenAPI(spec.Network),
		PreheatConfig:    preheatToOpenAPI(spec.Preheat),
		InstanceQuota:    spec.InstanceQuota,
		KS3MountConfig:   mountToOpenAPI(spec.Ks3MountConfig, runtime.KS3),
		KPFSMountConfig:  mountToOpenAPI(spec.KpfsMountConfig, runtime.KPFS),
		KlogEnabled:      spec.Klog != nil && spec.Klog.Enabled,
	}
}

func TemplateUpdateRequest(in *sandboxv1.SandboxTemplate, runtime RuntimeCredentials) openapi.UpdateTemplateRequest {
	req := TemplateCreateRequest(in, runtime)
	return openapi.UpdateTemplateRequest{
		TemplateID:            in.Status.TemplateID,
		CreateTemplateRequest: req,
	}
}

func SandboxStartRequest(in *sandboxv1.Sandbox, templateID string, runtime RuntimeCredentials) openapi.StartSandboxRequest {
	spec := in.Spec
	return openapi.StartSandboxRequest{
		TemplateID:      templateID,
		Timeout:         spec.TimeoutSeconds,
		Envs:            envsToOpenAPI(spec.Env),
		KS3MountConfig:  mountToOpenAPI(spec.Ks3MountConfig, runtime.KS3),
		KPFSMountConfig: mountToOpenAPI(spec.KpfsMountConfig, runtime.KPFS),
	}
}

type RuntimeCredentials struct {
	KS3  *credentials.RuntimeCredential
	KPFS *credentials.RuntimeCredential
}

func ApplyTemplateSpecFromOpenAPI(obj *sandboxv1.SandboxTemplate, remote openapi.Template) {
	obj.Spec.Description = remote.Description
	obj.Spec.Category = remote.TemplateCategory
	obj.Spec.Type = remote.TemplateType
	obj.Spec.Image = imageFromOpenAPI(remote.ImageConfig, obj.Spec.Image)
	obj.Spec.Command = remote.Command
	obj.Spec.Ports = append([]int(nil), remote.Ports...)
	obj.Spec.Resources = &sandboxv1.ResourceSpec{CPU: remote.CPU, MemoryGB: remote.Memory}
	obj.Spec.Env = envsFromOpenAPI(remote.Envs)
	obj.Spec.Network = networkFromOpenAPI(remote.NetworkConfig)
	obj.Spec.Preheat = preheatFromOpenAPI(remote.PreheatConfig)
	obj.Spec.InstanceQuota = remote.InstanceQuota
}

func ApplyTemplateStatusFromOpenAPI(obj *sandboxv1.SandboxTemplate, remote openapi.Template) {
	obj.Status.ObservedGeneration = obj.Generation
	obj.Status.TemplateID = remote.TemplateID
	obj.Status.Phase = TemplatePhase(remote.Status)
	obj.Status.RawStatus = remote.Status
	obj.Status.CanDelete = remote.CanDelete
	obj.Status.CreatedAt = metaTime(remote.CreatedAt)
	obj.Status.UpdatedAt = metaTime(remote.UpdatedAt)
	obj.Status.ExternalUpdatedAt = metaTime(remote.UpdatedAt)
	obj.Status.Klog = &sandboxv1.KlogStatus{ProjectName: remote.KlogProjectName, PoolName: remote.KlogPoolName}
	obj.Status.Quota = &sandboxv1.QuotaStatus{
		InstanceQuota:                remote.InstanceQuota,
		RemainingInstanceQuota:       remote.RemainingInstanceQuota,
		RemainingSystemInstanceQuota: remote.RemainingSystemInstanceQuota,
	}
	obj.Status.Preheat = &sandboxv1.PreheatStatus{
		Enabled:                 remote.PreheatConfig != nil && remote.PreheatConfig.Enabled,
		Number:                  preheatNumber(remote.PreheatConfig),
		PreheatedInstanceNumber: remote.PreheatedInstanceNumber,
	}
	if remote.CredentialAccessKeyIDMasked != "" {
		now := metav1.Now()
		obj.Status.CredentialDrift = &sandboxv1.CredentialDriftSet{
			KS3: &sandboxv1.CredentialDriftStatus{
				InSync:            "unknown",
				Source:            "OpenAPI",
				AccessKeyIDMasked: remote.CredentialAccessKeyIDMasked,
				ObservedAt:        &now,
				Reason:            "SecretNotReconciled",
			},
		}
	}
}

func ApplySandboxSpecFromOpenAPI(obj *sandboxv1.Sandbox, remote openapi.Sandbox) {
	if obj.Spec.Name == "" {
		obj.Spec.Name = obj.Name
	}
	if remote.Timeout > 0 {
		obj.Spec.TimeoutSeconds = remote.Timeout
	}
}

func ApplySandboxStatusFromOpenAPI(obj *sandboxv1.Sandbox, remote openapi.Sandbox) {
	obj.Status.ObservedGeneration = obj.Generation
	obj.Status.SandboxID = remote.SandboxID
	obj.Status.ExternalUpdatedAt = metaTime(remote.EndTime)
	obj.Status.Template = &sandboxv1.SandboxTemplateSummary{
		ID:       remote.TemplateID,
		Type:     remote.TemplateType,
		Category: remote.TemplateCategory,
	}
	obj.Status.Phase = SandboxPhase(remote.Status)
	obj.Status.RawStatus = remote.Status
	obj.Status.TimeoutSeconds = remote.Timeout
	obj.Status.CreateTime = metaTime(remote.CreateTime)
	obj.Status.EndTime = metaTime(remote.EndTime)
	obj.Status.Endpoint = remote.Endpoint
	if remote.CustomConfiguration != nil {
		obj.Status.CustomConfiguration = &sandboxv1.SandboxCustomConfiguration{
			ImageURL: remote.CustomConfiguration.ImageURL,
			Port:     remote.CustomConfiguration.Port,
			Command:  remote.CustomConfiguration.Command,
		}
	}
}

func TemplatePhase(raw string) sandboxv1.Phase {
	switch raw {
	case "Ready", "READY":
		return sandboxv1.PhaseReady
	case "":
		return sandboxv1.PhasePending
	default:
		return sandboxv1.PhaseUnknown
	}
}

func SandboxPhase(raw string) sandboxv1.Phase {
	switch raw {
	case "STARTING":
		return sandboxv1.PhaseStarting
	case "RUNNING":
		return sandboxv1.PhaseRunning
	case "KILLING":
		return sandboxv1.PhaseDeleting
	case "FAILED":
		return sandboxv1.PhaseFailed
	case "UNHEALTHY":
		return sandboxv1.PhaseUnhealthy
	case "PAUSED":
		return sandboxv1.PhasePaused
	case "RESUMING":
		return sandboxv1.PhaseResuming
	case "":
		return sandboxv1.PhaseUnknown
	default:
		return sandboxv1.PhaseUnknown
	}
}

func imageToOpenAPI(in *sandboxv1.ImageSpec) *openapi.ImageConfig {
	if in == nil {
		return nil
	}
	return &openapi.ImageConfig{
		Source:             in.Source,
		ImageURL:           in.ImageURL,
		ImageEndpoint:      in.ImageEndpoint,
		ImageNamespace:     in.ImageNamespace,
		ImageName:          in.ImageName,
		ImageTag:           in.ImageTag,
		RegistryInstanceID: in.RegistryInstanceID,
	}
}

func imageFromOpenAPI(in *openapi.ImageConfig, previous *sandboxv1.ImageSpec) *sandboxv1.ImageSpec {
	if in == nil {
		return previous
	}
	out := &sandboxv1.ImageSpec{}
	if previous != nil {
		out.CredentialRef = previous.CredentialRef
	}
	out.Source = in.Source
	out.ImageURL = in.ImageURL
	out.ImageEndpoint = in.ImageEndpoint
	out.ImageNamespace = in.ImageNamespace
	out.ImageName = in.ImageName
	out.ImageTag = in.ImageTag
	out.RegistryInstanceID = in.RegistryInstanceID
	return out
}

func networkToOpenAPI(in *sandboxv1.NetworkSpec) *openapi.NetworkConfig {
	if in == nil {
		return nil
	}
	out := &openapi.NetworkConfig{
		PublicNetworkEnable:        in.PublicNetworkEnable,
		PrivateNetworkEnable:       in.PrivateNetworkEnable,
		SharedInternetAccessEnable: in.SharedInternetAccessEnable,
	}
	if in.VPC != nil {
		out.VPCID = in.VPC.VPCID
		out.SubnetID = in.VPC.SubnetID
	}
	return out
}

func networkFromOpenAPI(in *openapi.NetworkConfig) *sandboxv1.NetworkSpec {
	if in == nil {
		return nil
	}
	return &sandboxv1.NetworkSpec{
		PublicNetworkEnable:        in.PublicNetworkEnable,
		PrivateNetworkEnable:       in.PrivateNetworkEnable,
		SharedInternetAccessEnable: in.SharedInternetAccessEnable,
		VPC: &sandboxv1.VPCSpec{
			VPCID:    in.VPCID,
			SubnetID: in.SubnetID,
		},
	}
}

func preheatToOpenAPI(in *sandboxv1.PreheatSpec) *openapi.PreheatConfig {
	if in == nil {
		return nil
	}
	return &openapi.PreheatConfig{Enabled: in.Enabled, Number: in.Number}
}

func preheatFromOpenAPI(in *openapi.PreheatConfig) *sandboxv1.PreheatSpec {
	if in == nil {
		return nil
	}
	return &sandboxv1.PreheatSpec{Enabled: in.Enabled, Number: in.Number}
}

func mountToOpenAPI(in *sandboxv1.MountConfig, cred *credentials.RuntimeCredential) *openapi.MountConfig {
	if in == nil {
		return nil
	}
	out := &openapi.MountConfig{
		Enabled:     in.Enabled,
		MountPoints: mountPointsToOpenAPI(in.MountPoints),
	}
	if cred != nil {
		out.AccessKey = cred.AccessKey
		out.SecretAccessKey = cred.SecretAccessKey
	}
	return out
}

func mountPointsToOpenAPI(in []sandboxv1.MountPoint) []openapi.MountPoint {
	out := make([]openapi.MountPoint, 0, len(in))
	for _, item := range in {
		out = append(out, openapi.MountPoint{
			BucketName:     item.BucketName,
			FileSystemName: item.FileSystemName,
			RemotePath:     item.RemotePath,
			LocalMountPath: item.LocalMountPath,
			ReadOnly:       item.ReadOnly,
		})
	}
	return out
}

func envsToOpenAPI(in []sandboxv1.EnvVar) []openapi.Env {
	out := make([]openapi.Env, 0, len(in))
	for _, item := range in {
		out = append(out, openapi.Env{Key: item.Key, Value: item.Value})
	}
	return out
}

func envsFromOpenAPI(in []openapi.Env) []sandboxv1.EnvVar {
	out := make([]sandboxv1.EnvVar, 0, len(in))
	for _, item := range in {
		out = append(out, sandboxv1.EnvVar{Key: item.Key, Value: item.Value})
	}
	return out
}

func resourceCPU(in *sandboxv1.ResourceSpec) int {
	if in == nil {
		return 0
	}
	return in.CPU
}

func resourceMemory(in *sandboxv1.ResourceSpec) int {
	if in == nil {
		return 0
	}
	return in.MemoryGB
}

func preheatNumber(in *openapi.PreheatConfig) int {
	if in == nil {
		return 0
	}
	return in.Number
}

func metaTime(t *time.Time) *metav1.Time {
	if t == nil {
		return nil
	}
	mt := metav1.NewTime(*t)
	return &mt
}

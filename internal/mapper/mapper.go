package mapper

import (
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
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
	tpl := templateSpec(spec)
	return openapi.CreateTemplateRequest{
		TemplateName:       in.Name,
		Description:        spec.Description,
		TemplateCategory:   templateAccess(spec.Access),
		TemplateType:       templateType(spec.Type),
		Image:              templateImageURL(tpl),
		ImageSource:        templateImageSource(tpl),
		CredentialServer:   registryServer(runtime.Registry),
		CredentialUsername: registryUsername(runtime.Registry),
		CredentialPassword: registryPassword(runtime.Registry),
		Command:            templateStartCommand(tpl),
		Ports:              templatePorts(tpl),
		CPU:                templateCPU(tpl),
		Memory:             templateMemoryMB(tpl),
		DiskSizeMB:         templateDiskMB(tpl),
		Envs:               templateEnv(tpl),
		NetworkConfig:      templateNetwork(tpl),
		TargetPoolSize:     templatePoolTargetSize(spec),
		KS3MountConfig:     templateMountToOpenAPI(tpl, runtime.KS3ByName, "ks3"),
		KPFSMountConfig:    templateMountToOpenAPI(tpl, runtime.KPFSByName, "kpfs"),
		KlogConfig:         templateKlogToOpenAPI(spec, runtime.Klog),
		SkillConfig:        templateSkillToOpenAPI(tpl),
		DataDisks:          templateDataDisks(tpl),
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
		TemplateID: templateID,
		Timeout:    spec.TimeoutSeconds,
		EnvVars:    envsToMap(spec.Env),
		Metadata:   sandboxMetadata(spec, runtime),
	}
}

type RuntimeCredentials struct {
	KS3        *credentials.RuntimeCredential
	KPFS       *credentials.RuntimeCredential
	KS3ByName  map[string]*credentials.RuntimeCredential
	KPFSByName map[string]*credentials.RuntimeCredential
	Registry   *credentials.RegistryCredential
	Klog       *credentials.RuntimeCredential
}

func ApplyTemplateSpecFromOpenAPI(obj *sandboxv1.SandboxTemplate, remote openapi.Template) {
	obj.Spec.Description = remote.Description
	obj.Spec.Access = displayAccess(remote.TemplateCategory)
	obj.Spec.Type = remote.TemplateType
	applyRuntimeSpecFromOpenAPI(obj, remote)
	if remote.TargetPoolSize > 0 {
		obj.Spec.Pool = &sandboxv1.TemplatePoolSpec{TargetSize: remote.TargetPoolSize}
	} else {
		obj.Spec.Pool = nil
	}
}

func ApplyTemplateStatusFromOpenAPI(obj *sandboxv1.SandboxTemplate, remote openapi.Template) {
	obj.Status.ObservedGeneration = obj.Generation
	obj.Status.TemplateID = remote.Identifier()
	obj.Status.Phase = TemplatePhase(remote.Status)
	obj.Status.RawStatus = remote.Status
	obj.Status.CanDelete = remote.CanDelete
	obj.Status.CreatedAt = metaTimeString(remote.CreatedAt)
	obj.Status.UpdatedAt = metaTimeString(remote.UpdatedAt)
	obj.Status.ExternalUpdatedAt = metaTimeString(remote.UpdatedAt)
	if remote.KlogConfig != nil {
		obj.Status.Klog = &sandboxv1.KlogStatus{ProjectName: remote.KlogConfig.ProjectName, PoolName: remote.KlogConfig.PoolNameContainer}
	}
	obj.Status.Quota = &sandboxv1.QuotaStatus{
		InstanceQuota:                remote.InstanceQuota,
		RemainingInstanceQuota:       remote.RemainingInstanceQuota,
		RemainingSystemInstanceQuota: remote.RemainingSystemInstanceQuota,
	}
	obj.Status.Preheat = &sandboxv1.PreheatStatus{
		Enabled:                 remote.TargetPoolSize > 0,
		Number:                  remote.TargetPoolSize,
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
	obj.Status.ExternalUpdatedAt = metaTimeString(remote.EndTime)
	obj.Status.Template = &sandboxv1.SandboxTemplateSummary{
		ID:       remote.TemplateID,
		Type:     remote.TemplateType,
		Category: remote.TemplateCategory,
	}
	obj.Status.Phase = SandboxPhase(remote.Status)
	obj.Status.RawStatus = remote.Status
	obj.Status.TimeoutSeconds = remote.Timeout
	obj.Status.CreateTime = metaTimeString(remote.CreateTime)
	obj.Status.EndTime = metaTimeString(remote.EndTime)
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
	case "Ready", "READY", "ready":
		return sandboxv1.PhaseReady
	case "creating", "CREATING":
		return sandboxv1.PhasePending
	case "error", "ERROR":
		return sandboxv1.PhaseFailed
	case "":
		return sandboxv1.PhasePending
	default:
		return sandboxv1.PhaseUnknown
	}
}

func SandboxPhase(raw string) sandboxv1.Phase {
	switch raw {
	case "STARTING", "starting":
		return sandboxv1.PhaseStarting
	case "RUNNING", "running":
		return sandboxv1.PhaseRunning
	case "KILLING", "killing":
		return sandboxv1.PhaseDeleting
	case "FAILED", "failed":
		return sandboxv1.PhaseFailed
	case "UNHEALTHY", "unhealthy":
		return sandboxv1.PhaseUnhealthy
	case "PAUSED", "paused":
		return sandboxv1.PhasePaused
	case "RESUMING", "resuming":
		return sandboxv1.PhaseResuming
	case "":
		return sandboxv1.PhaseUnknown
	default:
		return sandboxv1.PhaseUnknown
	}
}

func templateSpec(spec sandboxv1.SandboxTemplateSpec) *sandboxv1.RuntimeTemplateSpec {
	if spec.Template == nil {
		return nil
	}
	return &spec.Template.Spec
}

func templateImageURL(tpl *sandboxv1.RuntimeTemplateSpec) string {
	if tpl != nil && tpl.Image != nil {
		return tpl.Image.Image
	}
	return ""
}

func templateImageSource(tpl *sandboxv1.RuntimeTemplateSpec) string {
	if tpl != nil && tpl.Image != nil {
		return tpl.Image.Source
	}
	return ""
}

func templateStartCommand(tpl *sandboxv1.RuntimeTemplateSpec) string {
	if tpl != nil && tpl.StartCommand != "" {
		return tpl.StartCommand
	}
	return ""
}

func templatePorts(tpl *sandboxv1.RuntimeTemplateSpec) []int {
	if tpl == nil {
		return nil
	}
	out := make([]int, 0, len(tpl.Ports))
	for _, port := range tpl.Ports {
		if port.ContainerPort > 0 {
			out = append(out, port.ContainerPort)
		}
	}
	return out
}

func templateCPU(tpl *sandboxv1.RuntimeTemplateSpec) int {
	if tpl != nil && tpl.Resources != nil && tpl.Resources.CPU != "" {
		value, err := strconv.Atoi(strings.TrimSpace(tpl.Resources.CPU))
		if err == nil {
			return value
		}
	}
	return 0
}

func templateMemoryMB(tpl *sandboxv1.RuntimeTemplateSpec) int {
	if tpl != nil && tpl.Resources != nil && !tpl.Resources.Memory.IsZero() {
		return quantityMB(tpl.Resources.Memory.Value())
	}
	return 0
}

func templateDiskMB(tpl *sandboxv1.RuntimeTemplateSpec) int64 {
	if tpl == nil || tpl.Resources == nil || tpl.Resources.Disk.IsZero() {
		return 0
	}
	return int64(quantityMB(tpl.Resources.Disk.Value()))
}

func templateEnv(tpl *sandboxv1.RuntimeTemplateSpec) map[string]string {
	if tpl == nil || len(tpl.Env) == 0 {
		return nil
	}
	out := make(map[string]string, len(tpl.Env))
	for _, item := range tpl.Env {
		out[item.Name] = item.Value
	}
	return out
}

func templateNetwork(tpl *sandboxv1.RuntimeTemplateSpec) *openapi.NetworkConfig {
	if tpl == nil || tpl.NetworkConfig == nil {
		return nil
	}
	in := tpl.NetworkConfig
	return &openapi.NetworkConfig{
		PublicNetworkEnable:        in.EnablePublic,
		PrivateNetworkEnable:       in.EnablePrivate,
		SharedInternetAccessEnable: in.ChangeDefaultRoute,
		VPCID:                      in.UserVpcID,
		SubnetID:                   in.UserSubnetID,
		SecurityID:                 in.UserSgID,
		CIDRBlock:                  in.CIDRBlock,
		AvailabilityZone:           in.AvailabilityZone,
	}
}

func templatePoolTargetSize(spec sandboxv1.SandboxTemplateSpec) int {
	if spec.Pool != nil {
		return spec.Pool.TargetSize
	}
	return 0
}

func templateMountToOpenAPI(tpl *sandboxv1.RuntimeTemplateSpec, creds map[string]*credentials.RuntimeCredential, kind string) *openapi.MountConfig {
	if tpl == nil || len(tpl.Volumes) == 0 {
		return nil
	}
	out := &openapi.MountConfig{}
	var firstCred *credentials.RuntimeCredential
	for _, volume := range tpl.Volumes {
		if !strings.EqualFold(volume.Type, kind) {
			continue
		}
		switch kind {
		case "ks3":
			out.EnableKS3 = true
			if volume.KS3 == nil {
				continue
			}
			cred := creds[refName(volume.KS3.CredentialRef)]
			if firstCred == nil {
				firstCred = cred
			}
			out.MountPoints = append(out.MountPoints, openapi.MountPoint{
				BucketName:     volume.KS3.Bucket,
				BucketPath:     volume.KS3.Path,
				LocalMountPath: volume.MountPath,
				ReadOnly:       volume.ReadOnly,
			})
		case "kpfs":
			out.EnableKPFS = true
			if volume.KPFS == nil {
				continue
			}
			cred := creds[refName(volume.KPFS.CredentialRef)]
			if firstCred == nil {
				firstCred = cred
			}
			point := openapi.MountPoint{
				FileSystemName: volume.KPFS.FileSystem,
				RemotePath:     volume.KPFS.Path,
				LocalMountPath: volume.MountPath,
				ReadOnly:       volume.ReadOnly,
			}
			if cred != nil {
				point.Token = cred.Token
			}
			out.MountPoints = append(out.MountPoints, point)
		}
	}
	if len(out.MountPoints) == 0 {
		return nil
	}
	if firstCred != nil {
		out.Credential = &openapi.MountCredential{AccessKey: firstCred.AccessKey, SecretAccessKey: firstCred.SecretAccessKey}
	}
	return out
}

func templateKlogToOpenAPI(spec sandboxv1.SandboxTemplateSpec, cred *credentials.RuntimeCredential) *openapi.KlogConfig {
	if spec.Observability != nil && spec.Observability.Logging != nil {
		logging := spec.Observability.Logging
		out := &openapi.KlogConfig{
			Enabled:           logging.Enabled,
			ProjectName:       logging.ProjectName,
			KlogEndpoint:      logging.Endpoint,
			PoolNameContainer: logging.ContainerPoolName,
			PoolNameHost:      logging.HostPoolName,
			Rules:             append([]string(nil), logging.Rules...),
		}
		if cred != nil {
			out.AccessKey = cred.AccessKey
			out.SecretKey = cred.SecretAccessKey
		}
		return out
	}
	return nil
}

func templateSkillToOpenAPI(tpl *sandboxv1.RuntimeTemplateSpec) *openapi.SkillConfig {
	if tpl == nil || tpl.SkillConfig == nil {
		return nil
	}
	return &openapi.SkillConfig{
		Enable:            tpl.SkillConfig.Enable,
		SpaceID:           strings.Join(tpl.SkillConfig.SpaceIDs, ","),
		EnablePublicSkill: tpl.SkillConfig.EnablePublicSkill,
	}
}

func templateDataDisks(tpl *sandboxv1.RuntimeTemplateSpec) []openapi.DataDisk {
	if tpl == nil || len(tpl.DataDisks) == 0 {
		return nil
	}
	out := make([]openapi.DataDisk, 0, len(tpl.DataDisks))
	for _, disk := range tpl.DataDisks {
		out = append(out, openapi.DataDisk{
			Name:               disk.Name,
			Type:               disk.Type,
			SizeMB:             disk.SizeMB,
			DeleteWithInstance: disk.DeleteWithInstance,
			Path:               disk.Path,
		})
	}
	return out
}

func applyRuntimeSpecFromOpenAPI(obj *sandboxv1.SandboxTemplate, remote openapi.Template) {
	if obj.Spec.Template == nil {
		obj.Spec.Template = &sandboxv1.RuntimeTemplate{}
	}
	tpl := &obj.Spec.Template.Spec
	tpl.Image = &sandboxv1.TemplateImageSpec{Source: remote.ImageSource, Image: remote.Image}
	tpl.Resources = &sandboxv1.RuntimeResourceSpec{
		CPU:    strconv.Itoa(remote.CPU),
		Memory: *resourceFromMB(remote.Memory),
		Disk:   *resourceFromMB64(remote.DiskSizeMB),
	}
	tpl.Ports = portsFromOpenAPI(remote.Ports)
	tpl.StartCommand = remote.Command
	tpl.Env = templateEnvFromMap(remote.Envs)
	tpl.NetworkConfig = networkConfigFromOpenAPI(remote.NetworkConfig)
	tpl.SkillConfig = skillFromOpenAPI(remote.SkillConfig)
	tpl.DataDisks = dataDisksFromOpenAPI(remote.DataDisks)
}

func portsFromOpenAPI(in []int) []sandboxv1.ContainerPortSpec {
	out := make([]sandboxv1.ContainerPortSpec, 0, len(in))
	for _, port := range in {
		out = append(out, sandboxv1.ContainerPortSpec{ContainerPort: port, Protocol: "TCP"})
	}
	return out
}

func templateEnvFromMap(in map[string]string) []sandboxv1.TemplateEnvVar {
	out := make([]sandboxv1.TemplateEnvVar, 0, len(in))
	for key, value := range in {
		out = append(out, sandboxv1.TemplateEnvVar{Name: key, Value: value})
	}
	return out
}

func networkConfigFromOpenAPI(in *openapi.NetworkConfig) *sandboxv1.OpenAPINetworkConfig {
	if in == nil {
		return nil
	}
	return &sandboxv1.OpenAPINetworkConfig{
		EnablePublic:       in.PublicNetworkEnable,
		EnablePrivate:      in.PrivateNetworkEnable,
		CIDRBlock:          in.CIDRBlock,
		ChangeDefaultRoute: in.SharedInternetAccessEnable,
		UserVpcID:          in.VPCID,
		UserSgID:           in.SecurityID,
		UserSubnetID:       in.SubnetID,
		AvailabilityZone:   in.AvailabilityZone,
	}
}

func skillFromOpenAPI(in *openapi.SkillConfig) *sandboxv1.SkillConfig {
	if in == nil {
		return nil
	}
	var spaces []string
	if in.SpaceID != "" {
		spaces = strings.Split(in.SpaceID, ",")
	}
	return &sandboxv1.SkillConfig{Enable: in.Enable, SpaceIDs: spaces, EnablePublicSkill: in.EnablePublicSkill}
}

func dataDisksFromOpenAPI(in []openapi.DataDisk) []sandboxv1.DataDiskSpec {
	out := make([]sandboxv1.DataDiskSpec, 0, len(in))
	for _, disk := range in {
		out = append(out, sandboxv1.DataDiskSpec{Name: disk.Name, Type: disk.Type, SizeMB: disk.SizeMB, DeleteWithInstance: disk.DeleteWithInstance, Path: disk.Path})
	}
	return out
}

func registryServer(in *credentials.RegistryCredential) string {
	if in == nil {
		return ""
	}
	return in.Server
}

func registryUsername(in *credentials.RegistryCredential) string {
	if in == nil {
		return ""
	}
	return in.Username
}

func registryPassword(in *credentials.RegistryCredential) string {
	if in == nil {
		return ""
	}
	return in.Password
}

func refName(ref *sandboxv1.LocalObjectReference) string {
	if ref == nil {
		return ""
	}
	return ref.Name
}

func quantityMB(bytes int64) int {
	if bytes <= 0 {
		return 0
	}
	return int((bytes + 1024*1024 - 1) / (1024 * 1024))
}

func resourceFromMB(value int) *resource.Quantity {
	return resourceFromMB64(int64(value))
}

func resourceFromMB64(value int64) *resource.Quantity {
	q := resource.NewQuantity(value*1024*1024, resource.BinarySI)
	return q
}

func templateAccess(value string) string {
	switch strings.ToLower(value) {
	case "private":
		return "private"
	case "public":
		return "public"
	default:
		return value
	}
}

func displayAccess(value string) string {
	switch strings.ToLower(value) {
	case "private":
		return "Private"
	case "public":
		return "Public"
	default:
		return value
	}
}

func templateType(value string) string {
	switch strings.ToLower(value) {
	case "custom":
		return "custom"
	case "browser":
		return "browser"
	case "code":
		return "code"
	case "aio":
		return "AIO"
	default:
		return value
	}
}

func mountToOpenAPI(in *sandboxv1.MountConfig, cred *credentials.RuntimeCredential, kind string) *openapi.MountConfig {
	if in == nil {
		return nil
	}
	out := &openapi.MountConfig{
		MountPoints: mountPointsToOpenAPI(in.MountPoints),
	}
	switch kind {
	case "ks3":
		out.EnableKS3 = in.Enabled
	case "kpfs":
		out.EnableKPFS = in.Enabled
	}
	if cred != nil {
		out.Credential = &openapi.MountCredential{
			AccessKey:       cred.AccessKey,
			SecretAccessKey: cred.SecretAccessKey,
		}
	}
	return out
}

func mountPointsToOpenAPI(in []sandboxv1.MountPoint) []openapi.MountPoint {
	out := make([]openapi.MountPoint, 0, len(in))
	for _, item := range in {
		out = append(out, openapi.MountPoint{
			BucketName:     item.BucketName,
			BucketPath:     item.RemotePath,
			FileSystemName: item.FileSystemName,
			RemotePath:     item.RemotePath,
			LocalMountPath: item.LocalMountPath,
			ReadOnly:       item.ReadOnly,
		})
	}
	return out
}

func envsToMap(in []sandboxv1.EnvVar) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for _, item := range in {
		out[item.Key] = item.Value
	}
	return out
}

func envsFromMap(in map[string]string) []sandboxv1.EnvVar {
	out := make([]sandboxv1.EnvVar, 0, len(in))
	for key, value := range in {
		out = append(out, sandboxv1.EnvVar{Key: key, Value: value})
	}
	return out
}

func sandboxMetadata(spec sandboxv1.SandboxSpec, runtime RuntimeCredentials) map[string]interface{} {
	metadata := map[string]interface{}{}
	var mounts []map[string]interface{}
	mounts = appendVolumeMounts(mounts, "ks3", spec.Ks3MountConfig, runtime.KS3)
	mounts = appendVolumeMounts(mounts, "kpfs", spec.KpfsMountConfig, runtime.KPFS)
	if len(mounts) > 0 {
		metadata["volumeMounts"] = mounts
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func appendVolumeMounts(out []map[string]interface{}, kind string, cfg *sandboxv1.MountConfig, cred *credentials.RuntimeCredential) []map[string]interface{} {
	if cfg == nil || !cfg.Enabled {
		return out
	}
	for _, mount := range cfg.MountPoints {
		item := map[string]interface{}{
			"type":     kind,
			"target":   mount.LocalMountPath,
			"readOnly": mount.ReadOnly,
		}
		switch kind {
		case "ks3":
			item["source"] = mount.BucketName + mount.RemotePath
		case "kpfs":
			item["source"] = mount.FileSystemName + mount.RemotePath
		}
		if cred != nil {
			item["accessKeyId"] = cred.AccessKey
			item["accessKeySecret"] = cred.SecretAccessKey
			if cred.Token != "" {
				item["token"] = cred.Token
			}
		}
		out = append(out, item)
	}
	return out
}

func metaTimeString(value string) *metav1.Time {
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			mt := metav1.NewTime(parsed)
			return &mt
		}
	}
	return nil
}

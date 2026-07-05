package mapper

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sandboxv1 "sandbox-operator/api/v1alpha1"
	"sandbox-operator/internal/annotations"
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
		TemplateName:     in.Name,
		Description:      spec.Description,
		TemplateCategory: templateAccess(spec.Access),
		TemplateType:     templateType(spec.Type),
		ImageConfig:      templateImageConfig(tpl, runtime.Registry),
		Command:          templateStartCommand(tpl),
		Ports:            templatePorts(tpl),
		CPU:              templateCPU(tpl),
		Memory:           templateMemoryGB(tpl),
		Envs:             templateEnv(tpl),
		NetworkConfig:    templateNetwork(tpl),
		PreheatConfig:    templatePreheat(spec),
		KS3MountConfig:   templateMountToOpenAPI(tpl, "ks3", runtime.KS3ByName),
		KPFSMountConfig:  templateMountToOpenAPI(tpl, "kpfs", runtime.KPFSByName),
		KlogConfig:       templateKlogToOpenAPI(spec, runtime.Klog),
		SkillConfig:      templateSkillToOpenAPI(tpl),
		InstanceQuota:    templatePoolInstanceQuota(spec),
		AccessKey:        storageAccessKey(runtime),
		SecretAccessKey:  storageSecretAccessKey(runtime),
	}
}

func TemplateUpdateRequest(in *sandboxv1.SandboxTemplate, runtime RuntimeCredentials) openapi.UpdateTemplateRequest {
	req := TemplateCreateRequest(in, runtime)
	return openapi.UpdateTemplateRequest{
		TemplateID:            annotations.Get(in.Annotations, annotations.TemplateID),
		CreateTemplateRequest: req,
	}
}

func TemplateUpdateRequestFromDiff(in, old *sandboxv1.SandboxTemplate, runtime RuntimeCredentials) openapi.UpdateTemplateRequest {
	full := TemplateCreateRequest(in, runtime)
	req := openapi.UpdateTemplateRequest{TemplateID: annotations.Get(in.Annotations, annotations.TemplateID)}

	if old == nil {
		req.CreateTemplateRequest = full
		return req
	}
	if in.Spec.Description != old.Spec.Description {
		req.Description = full.Description
	}
	if in.Spec.Access != old.Spec.Access {
		req.TemplateCategory = full.TemplateCategory
	}
	if in.Spec.Type != old.Spec.Type {
		req.TemplateType = full.TemplateType
	}
	inTpl, oldTpl := templateSpec(in.Spec), templateSpec(old.Spec)
	if !reflect.DeepEqual(templateImage(inTpl), templateImage(oldTpl)) {
		req.ImageConfig = full.ImageConfig
	}
	if !reflect.DeepEqual(templatePortsSpec(inTpl), templatePortsSpec(oldTpl)) {
		req.Ports = full.Ports
	}
	if templateStartCommand(inTpl) != templateStartCommand(oldTpl) {
		req.Command = full.Command
	}
	if !reflect.DeepEqual(templateResources(inTpl), templateResources(oldTpl)) {
		req.CPU = full.CPU
		req.Memory = full.Memory
		if !templateDiskEqual(inTpl, oldTpl) {
			req.KecConfig = templateKecConfig(inTpl)
		}
	}
	if !reflect.DeepEqual(templateDataDiskSpec(inTpl), templateDataDiskSpec(oldTpl)) {
		req.KecConfig = templateKecConfig(inTpl)
	}
	if !reflect.DeepEqual(templateEnvSpec(inTpl), templateEnvSpec(oldTpl)) {
		req.Envs = full.Envs
	}
	if !reflect.DeepEqual(templateNetworkSpec(inTpl), templateNetworkSpec(oldTpl)) {
		req.NetworkConfig = full.NetworkConfig
	}
	if !reflect.DeepEqual(templateSkillSpec(inTpl), templateSkillSpec(oldTpl)) {
		req.SkillConfig = full.SkillConfig
	}
	ks3Changed := !reflect.DeepEqual(templateVolumes(inTpl, "ks3"), templateVolumes(oldTpl, "ks3"))
	kpfsChanged := !reflect.DeepEqual(templateVolumes(inTpl, "kpfs"), templateVolumes(oldTpl, "kpfs"))
	if ks3Changed || kpfsChanged {
		req.KS3MountConfig = templateMountUpdateToOpenAPI(inTpl, "ks3", runtime.KS3ByName)
		req.KPFSMountConfig = templateMountUpdateToOpenAPI(inTpl, "kpfs", runtime.KPFSByName)
		req.AccessKey = full.AccessKey
		req.SecretAccessKey = full.SecretAccessKey
	}
	if !reflect.DeepEqual(templateObservabilitySpec(inTpl), templateObservabilitySpec(oldTpl)) {
		req.KlogConfig = full.KlogConfig
	}
	if !reflect.DeepEqual(templatePoolSpec(inTpl), templatePoolSpec(oldTpl)) {
		req.PreheatConfig = full.PreheatConfig
	}
	return req
}

func TemplateRequestNeedsStorageCredential(req openapi.UpdateTemplateRequest) bool {
	return (req.KS3MountConfig != nil && req.KS3MountConfig.EnableKS3) ||
		(req.KPFSMountConfig != nil && req.KPFSMountConfig.EnableKPFS)
}

func CompleteTemplateUpdateRequestFromRemote(req *openapi.UpdateTemplateRequest, remote openapi.Template) error {
	if req == nil || req.KecConfig == nil || !req.KecConfig.Enabled {
		return nil
	}
	if remote.KecConfig != nil && req.KecConfig.InstanceType == "" {
		req.KecConfig.InstanceType = remote.KecConfig.InstanceType
	}
	if remote.KecConfig != nil && req.KecConfig.SystemDiskType == "" {
		req.KecConfig.SystemDiskType = remote.KecConfig.SystemDiskType
	}
	if remote.KecConfig != nil && req.KecConfig.SystemDiskSizeGB == 0 {
		req.KecConfig.SystemDiskSizeGB = remote.KecConfig.SystemDiskSizeGB
	}
	if req.KecConfig.InstanceType == "" || req.KecConfig.SystemDiskType == "" || req.KecConfig.SystemDiskSizeGB == 0 {
		return fmt.Errorf("updating disk requires existing OpenAPI KecConfig with InstanceType, SystemDiskType and SystemDiskSize")
	}
	return nil
}

func SandboxStartRequest(in *sandboxv1.Sandbox, templateID string, runtime RuntimeCredentials) openapi.StartSandboxRequest {
	spec := in.Spec
	return openapi.StartSandboxRequest{
		TemplateID:      templateID,
		Timeout:         spec.TimeoutSeconds,
		Envs:            envsToOpenAPI(spec.Env),
		KS3MountConfig:  mountToOpenAPI(spec.Ks3MountConfig, "ks3"),
		KPFSMountConfig: mountToOpenAPI(spec.KpfsMountConfig, "kpfs"),
		AccessKey:       sandboxStorageAccessKey(runtime),
		SecretAccessKey: sandboxStorageSecretAccessKey(runtime),
	}
}

func SandboxRequestNeedsStorageCredential(req openapi.StartSandboxRequest) bool {
	return (req.KS3MountConfig != nil && req.KS3MountConfig.EnableKS3) ||
		(req.KPFSMountConfig != nil && req.KPFSMountConfig.EnableKPFS)
}

func SandboxUpdateRequest(in *sandboxv1.Sandbox) openapi.UpdateSandboxRequest {
	return openapi.UpdateSandboxRequest{
		InstanceID: annotations.Get(in.Annotations, annotations.SandboxID),
		Timeout:    in.Spec.TimeoutSeconds,
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
	obj.Spec.Type = displayTemplateType(remote.TemplateType)
	applyRuntimeSpecFromOpenAPI(obj, remote)
	tpl := templateSpec(obj.Spec)
	if remote.TargetPoolSize() > 0 {
		tpl.Pool = &sandboxv1.TemplatePoolSpec{TargetSize: remote.TargetPoolSize()}
	} else if tpl != nil {
		tpl.Pool = nil
	}
}

func ApplyTemplateStatusFromOpenAPI(obj *sandboxv1.SandboxTemplate, remote openapi.Template) {
	obj.Status.Phase = TemplatePhase(remote.Status)
	obj.Status.CanDelete = remote.CanDelete
	obj.Status.CreatedAt = metaTimeString(remote.CreatedAt)
	obj.Status.UpdatedAt = metaTimeString(remote.UpdatedAt)
	obj.Status.ExternalUpdatedAt = metaTimeString(remote.UpdatedAt)
	obj.Status.Klog = nil
	obj.Status.Quota = &sandboxv1.QuotaStatus{
		InstanceQuota:                remote.InstanceQuota,
		RemainingInstanceQuota:       remote.RemainingInstanceQuota,
		RemainingSystemInstanceQuota: remote.RemainingSystemInstanceQuota,
	}
	obj.Status.Preheat = &sandboxv1.PreheatStatus{
		Enabled:                 remote.TargetPoolSize() > 0,
		Number:                  remote.TargetPoolSize(),
		PreheatedInstanceNumber: remote.PreheatedInstanceNumber(),
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
		if remote.Name() != "" {
			obj.Spec.Name = remote.Name()
		} else {
			obj.Spec.Name = obj.Name
		}
	}
	if obj.Spec.TemplateRef.ID == "" && remote.TemplateIdentifier() != "" {
		obj.Spec.TemplateRef.ID = remote.TemplateIdentifier()
	}
	if remote.Timeout > 0 {
		obj.Spec.TimeoutSeconds = remote.Timeout
	}
}

func ApplySandboxStatusFromOpenAPI(obj *sandboxv1.Sandbox, remote openapi.Sandbox) {
	obj.Status.ExternalUpdatedAt = metaTimeString(remote.EndTime)
	obj.Status.Phase = SandboxPhase(remote.Status)
	obj.Status.TimeoutSeconds = remote.Timeout
	obj.Status.CreateTime = metaTimeString(remote.CreateTime)
	obj.Status.EndTime = metaTimeString(remote.EndTime)
	obj.Status.Endpoint = firstNonEmpty(remote.Endpoint, remote.Domain)
	if urls := sandboxURLsFromOpenAPIURL(remote.URLs); urls != nil {
		obj.Status.URLs = urls
	}
	if accessURL := sandboxURLsFromOpenAPIURL(remote.AccessURL); accessURL != nil {
		obj.Status.AccessURL = accessURL
	}
	obj.Status.SdnsURLs = copyStringMap(remote.SdnsURLs)
	obj.Status.Env = sandboxEnvFromOpenAPI(remote.Envs)
	obj.Status.Volumes = sandboxVolumesFromOpenAPI(remote)
	obj.Status.CustomConfiguration = nil
	if remote.CustomConfiguration != nil {
		obj.Status.ImageURL = remote.CustomConfiguration.ImageURL
		obj.Status.Port = remote.CustomConfiguration.Port
		obj.Status.Command = remote.CustomConfiguration.Command
	} else {
		obj.Status.ImageURL = ""
		obj.Status.Port = 0
		obj.Status.Command = ""
	}
}

func ApplySandboxAccessURLFromOpenAPI(obj *sandboxv1.Sandbox, remote openapi.Sandbox) {
	if accessURL := sandboxURLsFromOpenAPIURL(remote.AccessURL); accessURL != nil {
		obj.Status.AccessURL = accessURL
	}
}

func sandboxEnvFromOpenAPI(in []openapi.Env) []sandboxv1.EnvVar {
	if len(in) == 0 {
		return nil
	}
	out := make([]sandboxv1.EnvVar, 0, len(in))
	for _, item := range in {
		out = append(out, sandboxv1.EnvVar{Key: item.Key, Value: item.Value})
	}
	return out
}

func sandboxURLsFromOpenAPIURL(in *openapi.URLs) *sandboxv1.SandboxURLs {
	if in == nil {
		return nil
	}
	out := &sandboxv1.SandboxURLs{
		CdpURL:      in.CdpURL,
		NoVncURL:    in.NoVncURL,
		CodeURL:     in.Code,
		AppURL:      in.AppURL,
		TerminalURL: in.TerminalURL,
		VscodeURL:   in.VscodeURL,
	}
	if out.CdpURL == "" && out.NoVncURL == "" && out.CodeURL == "" && out.AppURL == "" && out.TerminalURL == "" && out.VscodeURL == "" {
		return nil
	}
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
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

func templateImage(tpl *sandboxv1.RuntimeTemplateSpec) *sandboxv1.TemplateImageSpec {
	if tpl == nil {
		return nil
	}
	return tpl.Image
}

func templateImageURL(tpl *sandboxv1.RuntimeTemplateSpec) string {
	if tpl != nil && tpl.Image != nil {
		return tpl.Image.Image
	}
	return ""
}

func templateImageConfig(tpl *sandboxv1.RuntimeTemplateSpec, registry *credentials.RegistryCredential) *openapi.ImageConfig {
	if tpl != nil && tpl.Image != nil {
		imageURL, imageTag := splitImageTag(tpl.Image.Image)
		out := &openapi.ImageConfig{
			ImageSource: templateImageSource(tpl.Image.Source),
			ImageURL:    imageURL,
			ImageTag:    imageTag,
		}
		if !strings.EqualFold(out.ImageSource, "Public") {
			out.ImageEndpoint, out.ImageNamespace, out.ImageName = splitRegistryImage(imageURL, registryServer(registry))
			out.CredentialUsername = registryUsername(registry)
			out.CredentialPassword = registryPassword(registry)
		}
		return out
	}
	return nil
}

func templateImageSource(value string) string {
	switch strings.ToLower(value) {
	case "public":
		return "Public"
	case "personal":
		return "Personal"
	case "enterprise":
		return "Enterprise"
	default:
		return value
	}
}

func splitImageTag(image string) (string, string) {
	if image == "" {
		return "", ""
	}
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if lastColon > lastSlash {
		return image[:lastColon], image[lastColon+1:]
	}
	return image, ""
}

func splitRegistryImage(image, defaultEndpoint string) (endpoint, namespace, name string) {
	trimmed := strings.TrimPrefix(image, "https://")
	trimmed = strings.TrimPrefix(trimmed, "http://")
	parts := strings.Split(trimmed, "/")
	if len(parts) >= 3 {
		return parts[0], parts[1], strings.Join(parts[2:], "/")
	}
	if len(parts) == 2 {
		return defaultEndpoint, parts[0], parts[1]
	}
	return defaultEndpoint, "", image
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

func templatePortsSpec(tpl *sandboxv1.RuntimeTemplateSpec) []sandboxv1.ContainerPortSpec {
	if tpl == nil {
		return nil
	}
	return tpl.Ports
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

func templateResources(tpl *sandboxv1.RuntimeTemplateSpec) *sandboxv1.RuntimeResourceSpec {
	if tpl == nil {
		return nil
	}
	return tpl.Resources
}

func templateDiskEqual(a, b *sandboxv1.RuntimeTemplateSpec) bool {
	return templateDiskGB(a) == templateDiskGB(b)
}

func templateDiskGB(tpl *sandboxv1.RuntimeTemplateSpec) int64 {
	if tpl == nil || tpl.Resources == nil || tpl.Resources.Disk.IsZero() {
		return 0
	}
	return int64(quantityGB(tpl.Resources.Disk.Value()))
}

func templateDataDiskSpec(tpl *sandboxv1.RuntimeTemplateSpec) []sandboxv1.DataDiskSpec {
	if tpl == nil {
		return nil
	}
	return tpl.DataDisks
}

func templateMemoryMB(tpl *sandboxv1.RuntimeTemplateSpec) int {
	if tpl != nil && tpl.Resources != nil && !tpl.Resources.Memory.IsZero() {
		return quantityMB(tpl.Resources.Memory.Value())
	}
	return 0
}

func templateMemoryGB(tpl *sandboxv1.RuntimeTemplateSpec) int {
	if tpl != nil && tpl.Resources != nil && !tpl.Resources.Memory.IsZero() {
		return quantityGB(tpl.Resources.Memory.Value())
	}
	return 0
}

func templateEnv(tpl *sandboxv1.RuntimeTemplateSpec) []openapi.Env {
	if tpl == nil || len(tpl.Env) == 0 {
		return nil
	}
	out := make([]openapi.Env, 0, len(tpl.Env))
	for _, item := range tpl.Env {
		out = append(out, openapi.Env{Key: item.Name, Value: item.Value})
	}
	return out
}

func templateEnvSpec(tpl *sandboxv1.RuntimeTemplateSpec) []sandboxv1.TemplateEnvVar {
	if tpl == nil {
		return nil
	}
	return tpl.Env
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
		VPCConfiguration: &openapi.VPCConfig{
			VPCID:            in.UserVpcID,
			SubnetID:         in.UserSubnetID,
			CIDRBlock:        in.CIDRBlock,
			AvailabilityZone: in.AvailabilityZone,
		},
	}
}

func templateNetworkSpec(tpl *sandboxv1.RuntimeTemplateSpec) *sandboxv1.OpenAPINetworkConfig {
	if tpl == nil {
		return nil
	}
	return tpl.NetworkConfig
}

func templateSkillSpec(tpl *sandboxv1.RuntimeTemplateSpec) *sandboxv1.SkillConfig {
	if tpl == nil {
		return nil
	}
	return tpl.SkillConfig
}

func templatePoolSpec(tpl *sandboxv1.RuntimeTemplateSpec) *sandboxv1.TemplatePoolSpec {
	if tpl == nil {
		return nil
	}
	return tpl.Pool
}

func templateObservabilitySpec(tpl *sandboxv1.RuntimeTemplateSpec) *sandboxv1.ObservabilitySpec {
	if tpl == nil {
		return nil
	}
	return tpl.Observability
}

func templatePreheat(spec sandboxv1.SandboxTemplateSpec) *openapi.PreheatConfig {
	if pool := templatePoolSpec(templateSpec(spec)); pool != nil {
		return &openapi.PreheatConfig{PreheatEnable: pool.TargetSize > 0, PreheatNumber: pool.TargetSize}
	}
	return nil
}

func templatePoolInstanceQuota(spec sandboxv1.SandboxTemplateSpec) int {
	return 0
}

func templateKecConfig(tpl *sandboxv1.RuntimeTemplateSpec) *openapi.KecConfig {
	systemDiskSizeGB := templateDiskGB(tpl)
	dataDisks := templateDataDisks(tpl)
	if systemDiskSizeGB == 0 && len(dataDisks) == 0 {
		return nil
	}
	return &openapi.KecConfig{
		Enabled:          true,
		SystemDiskSizeGB: systemDiskSizeGB,
		DataDisks:        dataDisks,
	}
}

func templateVolumes(tpl *sandboxv1.RuntimeTemplateSpec, kind string) []sandboxv1.TemplateVolume {
	if tpl == nil || len(tpl.Volumes) == 0 {
		return nil
	}
	out := make([]sandboxv1.TemplateVolume, 0, len(tpl.Volumes))
	for _, volume := range tpl.Volumes {
		if strings.EqualFold(volume.Type, kind) {
			out = append(out, volume)
		}
	}
	return out
}

func templateMountToOpenAPI(tpl *sandboxv1.RuntimeTemplateSpec, kind string, creds map[string]*credentials.RuntimeCredential) *openapi.MountConfig {
	if tpl == nil || len(tpl.Volumes) == 0 {
		return nil
	}
	out := &openapi.MountConfig{}
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
			out.MountPoints = append(out.MountPoints, openapi.MountPoint{
				BucketName:     volume.KS3.Bucket,
				RemotePath:     volume.KS3.Path,
				LocalMountPath: volume.MountPath,
				ReadOnly:       volume.ReadOnly,
			})
		case "kpfs":
			out.EnableKPFS = true
			if volume.KPFS == nil {
				continue
			}
			point := openapi.MountPoint{
				FileSystemName: volume.KPFS.FileSystem,
				RemotePath:     volume.KPFS.Path,
				LocalMountPath: volume.MountPath,
				ReadOnly:       volume.ReadOnly,
			}
			if cred := creds[refName(volume.KPFS.CredentialRef)]; cred != nil {
				point.Token = cred.Token
			}
			out.KPFSMounts = append(out.KPFSMounts, point)
		}
	}
	if len(out.MountPoints) == 0 && len(out.KPFSMounts) == 0 {
		return nil
	}
	return out
}

func templateMountUpdateToOpenAPI(tpl *sandboxv1.RuntimeTemplateSpec, kind string, creds map[string]*credentials.RuntimeCredential) *openapi.MountConfig {
	out := templateMountToOpenAPI(tpl, kind, creds)
	if out != nil {
		return out
	}
	switch kind {
	case "ks3":
		return &openapi.MountConfig{EnableKS3: false}
	case "kpfs":
		return &openapi.MountConfig{EnableKPFS: false}
	default:
		return nil
	}
}

func templateKlogToOpenAPI(spec sandboxv1.SandboxTemplateSpec, cred *credentials.RuntimeCredential) *openapi.KlogConfig {
	if observability := templateObservabilitySpec(templateSpec(spec)); observability != nil && observability.Logging != nil {
		logging := observability.Logging
		return &openapi.KlogConfig{Enabled: logging.Enabled}
	}
	return nil
}

func templateSkillToOpenAPI(tpl *sandboxv1.RuntimeTemplateSpec) *openapi.SkillConfig {
	if tpl == nil || tpl.SkillConfig == nil {
		return nil
	}
	return &openapi.SkillConfig{
		Enable:            tpl.SkillConfig.Enable,
		SpaceIDs:          append([]string(nil), tpl.SkillConfig.SpaceIDs...),
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
			Type:               disk.Type,
			SizeGB:             (disk.SizeMB + 1023) / 1024,
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
	if remote.ImageSource() != "" || remote.ImageURL() != "" {
		tpl.Image = &sandboxv1.TemplateImageSpec{Source: remote.ImageSource(), Image: remote.ImageURL()}
	} else {
		tpl.Image = nil
	}
	if remote.CPU > 0 || remote.Memory > 0 || remote.DiskSizeMB() > 0 {
		tpl.Resources = &sandboxv1.RuntimeResourceSpec{}
		if remote.CPU > 0 {
			tpl.Resources.CPU = strconv.Itoa(remote.CPU)
		}
		if remote.Memory > 0 {
			tpl.Resources.Memory = *resourceFromGB(remote.Memory)
		}
		if remote.DiskSizeMB() > 0 {
			tpl.Resources.Disk = *resourceFromMB64(remote.DiskSizeMB())
		}
	} else {
		tpl.Resources = nil
	}
	if len(remote.Ports) > 0 {
		tpl.Ports = portsFromOpenAPI(remote.Ports)
	} else {
		tpl.Ports = nil
	}
	tpl.StartCommand = remote.Command
	if len(remote.Envs) > 0 {
		tpl.Env = templateEnvFromOpenAPI(remote.Envs)
	} else {
		tpl.Env = nil
	}
	tpl.NetworkConfig = networkConfigFromOpenAPI(remote.NetworkConfig)
	tpl.SkillConfig = skillFromOpenAPI(remote.SkillConfig)
	if remote.KecConfig != nil && len(remote.KecConfig.DataDisks) > 0 {
		tpl.DataDisks = dataDisksFromOpenAPI(remote.KecConfig.DataDisks)
	} else {
		tpl.DataDisks = nil
	}
	tpl.Volumes = volumesFromOpenAPI(remote)
	if remote.KlogConfig != nil {
		tpl.Observability = &sandboxv1.ObservabilitySpec{
			Logging: &sandboxv1.LoggingSpec{
				Enabled:           remote.KlogConfig.Enabled,
				ProjectName:       remote.KlogConfig.ProjectName,
				ContainerPoolName: remote.KlogConfig.PoolName,
			},
		}
	} else {
		tpl.Observability = nil
	}
}

func portsFromOpenAPI(in []int) []sandboxv1.ContainerPortSpec {
	out := make([]sandboxv1.ContainerPortSpec, 0, len(in))
	for _, port := range in {
		out = append(out, sandboxv1.ContainerPortSpec{ContainerPort: port, Protocol: "TCP"})
	}
	return out
}

func templateEnvFromOpenAPI(in []openapi.Env) []sandboxv1.TemplateEnvVar {
	out := make([]sandboxv1.TemplateEnvVar, 0, len(in))
	for _, item := range in {
		out = append(out, sandboxv1.TemplateEnvVar{Name: item.Key, Value: item.Value})
	}
	return out
}

func networkConfigFromOpenAPI(in *openapi.NetworkConfig) *sandboxv1.OpenAPINetworkConfig {
	if in == nil {
		return nil
	}
	out := &sandboxv1.OpenAPINetworkConfig{
		EnablePublic:       in.PublicNetworkEnable,
		EnablePrivate:      in.PrivateNetworkEnable,
		ChangeDefaultRoute: in.SharedInternetAccessEnable,
	}
	if in.VPCConfiguration != nil {
		out.CIDRBlock = in.VPCConfiguration.CIDRBlock
		out.UserVpcID = in.VPCConfiguration.VPCID
		out.UserSgID = in.VPCConfiguration.SecurityGroupID
		out.UserSubnetID = in.VPCConfiguration.SubnetID
		out.AvailabilityZone = in.VPCConfiguration.AvailabilityZone
	}
	return out
}

func skillFromOpenAPI(in *openapi.SkillConfig) *sandboxv1.SkillConfig {
	if in == nil {
		return nil
	}
	return &sandboxv1.SkillConfig{Enable: in.Enable, SpaceIDs: append([]string(nil), in.SpaceIDs...), EnablePublicSkill: in.EnablePublicSkill}
}

func dataDisksFromOpenAPI(in []openapi.DataDisk) []sandboxv1.DataDiskSpec {
	out := make([]sandboxv1.DataDiskSpec, 0, len(in))
	for _, disk := range in {
		out = append(out, sandboxv1.DataDiskSpec{Type: disk.Type, SizeMB: disk.SizeGB * 1024, DeleteWithInstance: disk.DeleteWithInstance, Path: disk.Path})
	}
	return out
}

func volumesFromOpenAPI(remote openapi.Template) []sandboxv1.TemplateVolume {
	return volumesFromMountConfigs(remote.KS3MountConfig, remote.KPFSMountConfig)
}

func sandboxVolumesFromOpenAPI(remote openapi.Sandbox) []sandboxv1.TemplateVolume {
	return volumesFromMountConfigs(remote.KS3MountConfig, remote.KPFSMountConfig)
}

func volumesFromMountConfigs(ks3Config, kpfsConfig *openapi.MountConfig) []sandboxv1.TemplateVolume {
	var out []sandboxv1.TemplateVolume
	if ks3Config != nil {
		for i, point := range ks3Config.Points() {
			out = append(out, sandboxv1.TemplateVolume{
				Name:      volumeName("ks3", point.BucketName, i),
				Type:      "KS3",
				MountPath: point.LocalMountPath,
				ReadOnly:  point.ReadOnly,
				KS3: &sandboxv1.KS3VolumeSource{
					Bucket: point.BucketName,
					Path:   point.RemotePath,
				},
			})
		}
	}
	if kpfsConfig != nil {
		for i, point := range kpfsConfig.Points() {
			out = append(out, sandboxv1.TemplateVolume{
				Name:      volumeName("kpfs", point.FileSystemName, i),
				Type:      "KPFS",
				MountPath: point.LocalMountPath,
				ReadOnly:  point.ReadOnly,
				KPFS: &sandboxv1.KPFSVolumeSource{
					FileSystem: point.FileSystemName,
					Path:       point.RemotePath,
				},
			})
		}
	}
	return out
}

func volumeName(prefix, value string, index int) string {
	value = strings.Trim(strings.ToLower(value), "-.")
	if value == "" {
		return prefix + "-" + strconv.Itoa(index)
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = strconv.Itoa(index)
	}
	return prefix + "-" + out
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

func storageAccessKey(runtime RuntimeCredentials) string {
	if runtime.KS3 != nil && runtime.KS3.AccessKey != "" {
		return runtime.KS3.AccessKey
	}
	if runtime.KPFS != nil && runtime.KPFS.AccessKey != "" {
		return runtime.KPFS.AccessKey
	}
	for _, cred := range runtime.KS3ByName {
		if cred != nil && cred.AccessKey != "" {
			return cred.AccessKey
		}
	}
	for _, cred := range runtime.KPFSByName {
		if cred != nil && cred.AccessKey != "" {
			return cred.AccessKey
		}
	}
	return ""
}

func storageSecretAccessKey(runtime RuntimeCredentials) string {
	if runtime.KS3 != nil && runtime.KS3.SecretAccessKey != "" {
		return runtime.KS3.SecretAccessKey
	}
	if runtime.KPFS != nil && runtime.KPFS.SecretAccessKey != "" {
		return runtime.KPFS.SecretAccessKey
	}
	for _, cred := range runtime.KS3ByName {
		if cred != nil && cred.SecretAccessKey != "" {
			return cred.SecretAccessKey
		}
	}
	for _, cred := range runtime.KPFSByName {
		if cred != nil && cred.SecretAccessKey != "" {
			return cred.SecretAccessKey
		}
	}
	return ""
}

func sandboxStorageAccessKey(runtime RuntimeCredentials) string {
	if runtime.KS3 != nil && runtime.KS3.AccessKey != "" {
		return runtime.KS3.AccessKey
	}
	if runtime.KPFS != nil {
		return runtime.KPFS.AccessKey
	}
	return ""
}

func sandboxStorageSecretAccessKey(runtime RuntimeCredentials) string {
	if runtime.KS3 != nil && runtime.KS3.SecretAccessKey != "" {
		return runtime.KS3.SecretAccessKey
	}
	if runtime.KPFS != nil {
		return runtime.KPFS.SecretAccessKey
	}
	return ""
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

func quantityGB(bytes int64) int {
	if bytes <= 0 {
		return 0
	}
	return int((bytes + 1024*1024*1024 - 1) / (1024 * 1024 * 1024))
}

func resourceFromMB(value int) *resource.Quantity {
	return resourceFromMB64(int64(value))
}

func resourceFromGB(value int) *resource.Quantity {
	q := resource.NewQuantity(int64(value)*1024*1024*1024, resource.BinarySI)
	return q
}

func resourceFromMB64(value int64) *resource.Quantity {
	q := resource.NewQuantity(value*1024*1024, resource.BinarySI)
	return q
}

func templateAccess(value string) string {
	switch strings.ToLower(value) {
	case "private":
		return "Private"
	case "public":
		return "Public"
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
		return "Custom"
	case "browser":
		return "Browser"
	case "code", "codeinterpreter":
		return "CodeInterpreter"
	case "aio", "all-in-one", "allinone":
		return "All-in-one"
	default:
		return value
	}
}

func displayTemplateType(value string) string {
	switch strings.ToLower(value) {
	case "aio", "all-in-one", "allinone":
		return "AIO"
	case "browser":
		return "Browser"
	case "code", "codeinterpreter":
		return "Code"
	case "custom":
		return "Custom"
	default:
		return value
	}
}

func mountToOpenAPI(in *sandboxv1.MountConfig, kind string) *openapi.MountConfig {
	if in == nil {
		return nil
	}
	out := &openapi.MountConfig{}
	switch kind {
	case "ks3":
		out.EnableKS3 = in.Enabled
		out.MountPoints = mountPointsToOpenAPI(in.MountPoints)
	case "kpfs":
		out.EnableKPFS = in.Enabled
		out.KPFSMounts = mountPointsToOpenAPI(in.MountPoints)
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
	if len(in) == 0 {
		return nil
	}
	out := make([]openapi.Env, 0, len(in))
	for _, item := range in {
		out = append(out, openapi.Env{Key: item.Key, Value: item.Value})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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

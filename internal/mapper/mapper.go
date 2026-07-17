package mapper

import (
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
		KecConfig:        templateKecConfig(tpl),
		Envs:             templateEnv(tpl),
		NetworkConfig:    templateNetwork(tpl),
		PreheatConfig:    templatePreheat(spec),
		KS3MountConfig:   mountToOpenAPI(tpl.Ks3MountConfig, "ks3"),
		KPFSMountConfig:  mountToOpenAPI(tpl.KpfsMountConfig, "kpfs"),
		KlogConfig:       templateKlogToOpenAPI(spec),
		SkillConfig:      templateSkillToOpenAPI(tpl),
		InstanceQuota:    templatePoolInstanceQuota(spec),
		AccessKey:        storageAccessKey(runtime),
		SecretAccessKey:  storageSecretAccessKey(runtime),
	}
}

func SandboxInlineTemplateObject(in *sandboxv1.Sandbox) *sandboxv1.SandboxTemplate {
	if in == nil || in.Spec.Template == nil {
		return nil
	}
	inline := in.Spec.Template
	name := inline.Name
	if name == "" {
		name = in.Name
		if name == "" {
			name = "inline-template"
		} else {
			name += "-inline-template"
		}
	}
	return &sandboxv1.SandboxTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: in.Namespace,
			Name:      name,
		},
		Spec: sandboxv1.SandboxTemplateSpec{
			Description:          inline.Description,
			Type:                 inline.Type,
			Access:               inline.Access,
			OpenAPICredentialRef: in.Spec.OpenAPICredentialRef,
			Template: &sandboxv1.RuntimeTemplate{
				Spec: inline.Spec,
			},
		},
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
	if templateCPU(inTpl) != templateCPU(oldTpl) {
		req.CPU = full.CPU
	}
	if templateMemoryGB(inTpl) != templateMemoryGB(oldTpl) {
		req.Memory = full.Memory
	}
	if !reflect.DeepEqual(templateInstanceSpecs(inTpl), templateInstanceSpecs(oldTpl)) {
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
	ks3Changed := !reflect.DeepEqual(inTpl.Ks3MountConfig, oldTpl.Ks3MountConfig)
	kpfsChanged := !reflect.DeepEqual(inTpl.KpfsMountConfig, oldTpl.KpfsMountConfig)
	if ks3Changed || kpfsChanged {
		req.KS3MountConfig = mountUpdateToOpenAPI(inTpl.Ks3MountConfig, "ks3")
		req.KPFSMountConfig = mountUpdateToOpenAPI(inTpl.KpfsMountConfig, "kpfs")
		req.AccessKey = full.AccessKey
		req.SecretAccessKey = full.SecretAccessKey
	}
	if !reflect.DeepEqual(templateObservabilitySpec(inTpl), templateObservabilitySpec(oldTpl)) {
		req.KlogConfig = full.KlogConfig
	}
	if !templateAccessIsPublic(in.Spec.Access) && !reflect.DeepEqual(templatePoolSpec(inTpl), templatePoolSpec(oldTpl)) {
		req.PreheatConfig = full.PreheatConfig
	}
	return req
}

func TemplateRequestNeedsStorageCredential(req openapi.UpdateTemplateRequest) bool {
	return (req.KS3MountConfig != nil && req.KS3MountConfig.EnableKS3) ||
		(req.KPFSMountConfig != nil && req.KPFSMountConfig.EnableKPFS)
}

func TemplateCreateRequestNeedsStorageCredential(req openapi.CreateTemplateRequest) bool {
	return (req.KS3MountConfig != nil && req.KS3MountConfig.EnableKS3) ||
		(req.KPFSMountConfig != nil && req.KPFSMountConfig.EnableKPFS)
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
	Storage  *credentials.RuntimeCredential
	Registry *credentials.RegistryCredential
}

func ApplyTemplateSpecFromOpenAPI(obj *sandboxv1.SandboxTemplate, remote openapi.Template) {
	obj.Spec.Description = remote.Description
	obj.Spec.Access = displayAccess(remote.TemplateCategory)
	obj.Spec.Type = displayTemplateType(remote.TemplateType)
	applyRuntimeSpecFromOpenAPI(obj, remote)
	tpl := templateSpec(obj.Spec)
	if templateAccessIsPublic(obj.Spec.Access) {
		if tpl != nil {
			tpl.Pool = nil
		}
	} else if remote.TargetPoolSize() > 0 {
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
	if templateAccessIsPublic(displayAccess(remote.TemplateCategory)) {
		obj.Status.Preheat = nil
	} else {
		obj.Status.Preheat = &sandboxv1.PreheatStatus{
			Enabled:                 remote.TargetPoolSize() > 0,
			Number:                  remote.TargetPoolSize(),
			PreheatedInstanceNumber: remote.PreheatedInstanceNumber(),
		}
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
	if obj.Spec.Template == nil && obj.Spec.TemplateRef.ID == "" && remote.TemplateIdentifier() != "" {
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
	obj.Status.Ks3MountConfig = mountConfigFromOpenAPI("ks3", remote.KS3MountConfig)
	obj.Status.KpfsMountConfig = mountConfigFromOpenAPI("kpfs", remote.KPFSMountConfig)
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
	if tpl != nil && tpl.KecConfig != nil && tpl.KecConfig.CPU != "" {
		value, err := strconv.Atoi(strings.TrimSpace(tpl.KecConfig.CPU))
		if err == nil {
			return value
		}
	}
	return 0
}

func templateInstanceSpecs(tpl *sandboxv1.RuntimeTemplateSpec) []sandboxv1.KecInstanceSpec {
	if tpl == nil || tpl.KecConfig == nil {
		return nil
	}
	return tpl.KecConfig.InstanceSpecs
}

func templateMemoryMB(tpl *sandboxv1.RuntimeTemplateSpec) int {
	if tpl != nil && tpl.KecConfig != nil && !tpl.KecConfig.Memory.IsZero() {
		return quantityMB(tpl.KecConfig.Memory.Value())
	}
	return 0
}

func templateMemoryGB(tpl *sandboxv1.RuntimeTemplateSpec) int {
	if tpl != nil && tpl.KecConfig != nil && !tpl.KecConfig.Memory.IsZero() {
		return quantityGB(tpl.KecConfig.Memory.Value())
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
	if tpl == nil || tpl.NetworkConfig == nil || *tpl.NetworkConfig == (sandboxv1.OpenAPINetworkConfig{}) {
		// OpenAPI requires explicit shared internet access for public-only networking.
		return &openapi.NetworkConfig{
			PublicNetworkEnable:        true,
			SharedInternetAccessEnable: true,
		}
	}
	in := tpl.NetworkConfig
	sharedInternetAccessEnable := in.SharedInternetAccessEnable
	if in.EnablePublic && !in.EnablePrivate {
		sharedInternetAccessEnable = true
	} else if !in.EnablePublic && in.EnablePrivate {
		sharedInternetAccessEnable = false
	}
	return &openapi.NetworkConfig{
		PublicNetworkEnable:        in.EnablePublic,
		PrivateNetworkEnable:       in.EnablePrivate,
		SharedInternetAccessEnable: sharedInternetAccessEnable,
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
	if templateAccessIsPublic(spec.Access) {
		return nil
	}
	if pool := templatePoolSpec(templateSpec(spec)); pool != nil {
		return &openapi.PreheatConfig{PreheatEnable: pool.TargetSize > 0, PreheatNumber: pool.TargetSize}
	}
	return nil
}

func templatePoolInstanceQuota(spec sandboxv1.SandboxTemplateSpec) int {
	return 0
}

func templateAccessIsPublic(value string) bool {
	return strings.EqualFold(value, "Public")
}

func templateKecConfig(tpl *sandboxv1.RuntimeTemplateSpec) *openapi.KecConfig {
	if specs := templateInstanceSpecs(tpl); len(specs) > 0 {
		return &openapi.KecConfig{
			Enabled:       true,
			InstanceSpecs: instanceSpecsToOpenAPI(specs),
		}
	}
	return nil
}

func instanceSpecsToOpenAPI(in []sandboxv1.KecInstanceSpec) []openapi.InstanceSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]openapi.InstanceSpec, 0, len(in))
	for _, spec := range in {
		out = append(out, openapi.InstanceSpec{
			InstanceType: spec.InstanceType,
			SystemDisk:   systemDiskToOpenAPI(spec.SystemDisk),
			DataDisks:    dataDiskSpecsToOpenAPI(spec.DataDisks),
		})
	}
	return out
}

func systemDiskToOpenAPI(in *sandboxv1.SystemDiskSpec) *openapi.SystemDisk {
	if in == nil {
		return nil
	}
	return &openapi.SystemDisk{
		Type:   in.Type,
		SizeGB: int64(quantityGB(in.Size.Value())),
	}
}

func mountConfigFromOpenAPI(kind string, cfg *openapi.MountConfig) *sandboxv1.MountConfig {
	if cfg == nil {
		return nil
	}
	enabled := false
	var points []sandboxv1.MountPoint
	switch kind {
	case "ks3":
		enabled = cfg.EnableKS3
		for _, p := range cfg.MountPoints {
			points = append(points, sandboxv1.MountPoint{
				BucketName:     p.BucketName,
				RemotePath:     p.RemotePath,
				LocalMountPath: p.LocalMountPath,
				ReadOnly:       p.ReadOnly,
			})
		}
	case "kpfs":
		enabled = cfg.EnableKPFS
		for _, p := range cfg.KPFSMounts {
			points = append(points, sandboxv1.MountPoint{
				FileSystemName: p.FileSystemName,
				RemotePath:     p.RemotePath,
				LocalMountPath: p.LocalMountPath,
				ReadOnly:       p.ReadOnly,
			})
		}
	}
	if !enabled && len(points) == 0 {
		return nil
	}
	return &sandboxv1.MountConfig{Enabled: enabled, MountPoints: points}
}

func templateKlogToOpenAPI(spec sandboxv1.SandboxTemplateSpec) *openapi.KlogConfig {
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

func dataDiskSpecsToOpenAPI(in []sandboxv1.DataDiskSpec) []openapi.DataDisk {
	if len(in) == 0 {
		return nil
	}
	out := make([]openapi.DataDisk, 0, len(in))
	for _, disk := range in {
		out = append(out, openapi.DataDisk{
			Type:               disk.Type,
			SizeGB:             int64(quantityGB(disk.Size.Value())),
			DeleteWithInstance: disk.DeleteWithInstance,
			Path:               disk.Path,
			FsType:             disk.FsType,
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
	tpl.KecConfig = runtimeKecConfigFromOpenAPI(remote)
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
	tpl.Ks3MountConfig = mountConfigFromOpenAPI("ks3", remote.KS3MountConfig)
	tpl.KpfsMountConfig = mountConfigFromOpenAPI("kpfs", remote.KPFSMountConfig)
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
		EnablePublic:               in.PublicNetworkEnable,
		EnablePrivate:              in.PrivateNetworkEnable,
		SharedInternetAccessEnable: in.SharedInternetAccessEnable,
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
		item := sandboxv1.DataDiskSpec{
			Type:               disk.Type,
			DeleteWithInstance: disk.DeleteWithInstance,
			Path:               disk.Path,
			FsType:             disk.FsType,
		}
		if disk.SizeGB > 0 {
			item.Size = *resourceFromGB(int(disk.SizeGB))
		}
		out = append(out, item)
	}
	return out
}

func runtimeKecConfigFromOpenAPI(remote openapi.Template) *sandboxv1.RuntimeKecConfig {
	out := &sandboxv1.RuntimeKecConfig{}
	if remote.CPU > 0 {
		out.CPU = strconv.Itoa(remote.CPU)
	}
	if remote.Memory > 0 {
		out.Memory = *resourceFromGB(remote.Memory)
	}
	if remote.KecConfig != nil && len(remote.KecConfig.InstanceSpecs) > 0 {
		out.InstanceSpecs = instanceSpecsFromOpenAPI(remote.KecConfig.InstanceSpecs)
		if out.CPU == "" {
			if cpu := firstInstanceSpecCPU(remote.KecConfig.InstanceSpecs); cpu > 0 {
				out.CPU = strconv.Itoa(cpu)
			}
		}
		if out.Memory.IsZero() {
			if memory := firstInstanceSpecMemory(remote.KecConfig.InstanceSpecs); memory > 0 {
				out.Memory = *resourceFromGB(memory)
			}
		}
	}
	if out.CPU == "" && out.Memory.IsZero() && len(out.InstanceSpecs) == 0 {
		return nil
	}
	return out
}

func instanceSpecsFromOpenAPI(in []openapi.InstanceSpec) []sandboxv1.KecInstanceSpec {
	out := make([]sandboxv1.KecInstanceSpec, 0, len(in))
	for _, spec := range in {
		item := sandboxv1.KecInstanceSpec{
			InstanceType: spec.InstanceType,
			SystemDisk:   systemDiskFromOpenAPI(spec.SystemDisk),
			DataDisks:    dataDisksFromOpenAPI(spec.DataDisks),
		}
		out = append(out, item)
	}
	return out
}

func firstInstanceSpecCPU(in []openapi.InstanceSpec) int {
	for _, spec := range in {
		if spec.CPU > 0 {
			return spec.CPU
		}
	}
	return 0
}

func firstInstanceSpecMemory(in []openapi.InstanceSpec) int {
	for _, spec := range in {
		if spec.Memory > 0 {
			return spec.Memory
		}
	}
	return 0
}

func systemDiskFromOpenAPI(in *openapi.SystemDisk) *sandboxv1.SystemDiskSpec {
	if in == nil {
		return nil
	}
	out := &sandboxv1.SystemDiskSpec{Type: in.Type}
	if in.SizeGB > 0 {
		out.Size = *resourceFromGB(int(in.SizeGB))
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

func storageAccessKey(runtime RuntimeCredentials) string {
	if runtime.Storage != nil {
		return runtime.Storage.AccessKey
	}
	return ""
}

func storageSecretAccessKey(runtime RuntimeCredentials) string {
	if runtime.Storage != nil {
		return runtime.Storage.SecretAccessKey
	}
	return ""
}

func sandboxStorageAccessKey(runtime RuntimeCredentials) string {
	if runtime.Storage != nil {
		return runtime.Storage.AccessKey
	}
	return ""
}

func sandboxStorageSecretAccessKey(runtime RuntimeCredentials) string {
	if runtime.Storage != nil {
		return runtime.Storage.SecretAccessKey
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

func mountUpdateToOpenAPI(cfg *sandboxv1.MountConfig, kind string) *openapi.MountConfig {
	if cfg != nil && cfg.Enabled {
		return mountToOpenAPI(cfg, kind)
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

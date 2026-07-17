package mapper

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sandboxv1 "sandbox-operator/api/v1alpha1"
	"sandbox-operator/internal/credentials"
	"sandbox-operator/internal/openapi"
)

func TestTemplateTypeMapping(t *testing.T) {
	cases := map[string]string{
		"AIO":             "All-in-one",
		"aio":             "All-in-one",
		"All-in-one":      "All-in-one",
		"allinone":        "All-in-one",
		"BROWSER":         "Browser",
		"browser":         "Browser",
		"Browser":         "Browser",
		"CODE":            "CodeInterpreter",
		"code":            "CodeInterpreter",
		"Code":            "CodeInterpreter",
		"CodeInterpreter": "CodeInterpreter",
		"CUSTOM":          "Custom",
		"custom":          "Custom",
		"Custom":          "Custom",
	}
	for in, want := range cases {
		if got := templateType(in); got != want {
			t.Fatalf("templateType(%q) = %q, want %q", in, got, want)
		}
	}

	displayCases := map[string]string{
		"All-in-one":      "AIO",
		"AIO":             "AIO",
		"browser":         "Browser",
		"Browser":         "Browser",
		"code":            "Code",
		"CodeInterpreter": "Code",
		"custom":          "Custom",
		"Custom":          "Custom",
	}
	for in, want := range displayCases {
		if got := displayTemplateType(in); got != want {
			t.Fatalf("displayTemplateType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApplyTemplateSpecDisplaysCanonicalCRType(t *testing.T) {
	var obj sandboxv1.SandboxTemplate
	ApplyTemplateSpecFromOpenAPI(&obj, openapi.Template{TemplateType: "browser", TemplateCategory: "Private"})
	if obj.Spec.Type != "Browser" {
		t.Fatalf("synced template type = %q, want Browser", obj.Spec.Type)
	}
}

func TestTemplateUpdateRequestFromDiffOnlySendsChangedTopLevelFields(t *testing.T) {
	old := templateWithKS3("old description", "/mnt/old")
	next := templateWithKS3("old description", "/mnt/old")
	next.Spec.Description = "new description"

	req := TemplateUpdateRequestFromDiff(next, old, RuntimeCredentials{})
	if req.Description != "new description" {
		t.Fatalf("description diff was not included: %#v", req)
	}
	if req.KS3MountConfig != nil || req.AccessKey != "" || req.SecretAccessKey != "" {
		t.Fatalf("unchanged mount config should not be included: %#v", req)
	}

	changedMount := templateWithKS3("old description", "/mnt/new")
	req = TemplateUpdateRequestFromDiff(changedMount, old, RuntimeCredentials{
		Storage: &credentials.RuntimeCredential{AccessKey: "ak", SecretAccessKey: "sk"},
	})
	if req.KS3MountConfig == nil || !req.KS3MountConfig.EnableKS3 {
		t.Fatalf("changed mount config should be included: %#v", req)
	}
	if req.AccessKey != "ak" || req.SecretAccessKey != "sk" {
		t.Fatalf("mount credential should be included, got ak=%q sk=%q", req.AccessKey, req.SecretAccessKey)
	}

	deletedMount := templateWithKS3("old description", "/mnt/old")
	deletedMount.Spec.Template.Spec.Ks3MountConfig = nil
	req = TemplateUpdateRequestFromDiff(deletedMount, old, RuntimeCredentials{})
	if req.KS3MountConfig == nil || req.KS3MountConfig.EnableKS3 {
		t.Fatalf("deleted KS3 mount must send Ks3Enable=false: %#v", req.KS3MountConfig)
	}
	if req.KPFSMountConfig == nil || req.KPFSMountConfig.EnableKPFS {
		t.Fatalf("mount diff must include KpfsEnable=false target state: %#v", req.KPFSMountConfig)
	}
}

func TestTemplateCreateRequestSendsKecInstanceSpecs(t *testing.T) {
	obj := templateWithKS3("description", "/mnt/data")
	obj.Spec.Template.Spec.KecConfig = &sandboxv1.RuntimeKecConfig{
		CPU:    "2",
		Memory: resource.MustParse("4Gi"),
		InstanceSpecs: []sandboxv1.KecInstanceSpec{
			{
				InstanceType: "S6.2B",
				SystemDisk:   &sandboxv1.SystemDiskSpec{Type: "SSD3.0", Size: resource.MustParse("20Gi")},
				DataDisks: []sandboxv1.DataDiskSpec{{
					Type:               "SSD3.0",
					Size:               resource.MustParse("50Gi"),
					DeleteWithInstance: true,
					Path:               "/data",
					FsType:             "ext4",
				}},
			},
			{
				InstanceType: "S6.2A",
				SystemDisk:   &sandboxv1.SystemDiskSpec{Type: "SSD3.0", Size: resource.MustParse("20Gi")},
				DataDisks: []sandboxv1.DataDiskSpec{{
					Type:               "SSD3.0",
					Size:               resource.MustParse("100Gi"),
					DeleteWithInstance: true,
					Path:               "/data2",
					FsType:             "ext4",
				}},
			},
		},
	}

	req := TemplateCreateRequest(obj, RuntimeCredentials{})
	if req.CPU != 2 || req.Memory != 4 {
		t.Fatalf("create request should send global CPU/Memory, got cpu=%d memory=%d", req.CPU, req.Memory)
	}
	if req.KecConfig == nil || !req.KecConfig.Enabled || len(req.KecConfig.InstanceSpecs) != 2 {
		t.Fatalf("create request should include KecConfig.InstanceSpecs: %#v", req.KecConfig)
	}
	first := req.KecConfig.InstanceSpecs[0]
	if first.InstanceType != "S6.2B" || first.SystemDisk == nil || first.SystemDisk.Type != "SSD3.0" || first.SystemDisk.SizeGB != 20 {
		t.Fatalf("first instance spec mismatch: %#v", first)
	}
	if len(first.DataDisks) != 1 || first.DataDisks[0].SizeGB != 50 || first.DataDisks[0].FsType != "ext4" {
		t.Fatalf("first instance spec data disk mismatch: %#v", first.DataDisks)
	}
}

func TestTemplateNetworkDefaultsSharedInternetForSingleNetwork(t *testing.T) {
	cases := []struct {
		name          string
		networkConfig *sandboxv1.OpenAPINetworkConfig
		wantPublic    bool
		wantPrivate   bool
		wantShared    bool
	}{
		{
			name:       "omitted network configuration defaults to public access",
			wantPublic: true,
			wantShared: true,
		},
		{
			name:          "empty network configuration defaults to public access",
			networkConfig: &sandboxv1.OpenAPINetworkConfig{},
			wantPublic:    true,
			wantShared:    true,
		},
		{
			name:          "public only defaults shared internet to enabled",
			networkConfig: &sandboxv1.OpenAPINetworkConfig{EnablePublic: true},
			wantPublic:    true,
			wantShared:    true,
		},
		{
			name:          "private only defaults shared internet to disabled",
			networkConfig: &sandboxv1.OpenAPINetworkConfig{EnablePrivate: true, SharedInternetAccessEnable: true},
			wantPrivate:   true,
			wantShared:    false,
		},
		{
			name:          "dual network keeps explicit shared internet setting",
			networkConfig: &sandboxv1.OpenAPINetworkConfig{EnablePublic: true, EnablePrivate: true},
			wantPublic:    true,
			wantPrivate:   true,
			wantShared:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			network := templateNetwork(&sandboxv1.RuntimeTemplateSpec{
				NetworkConfig: tc.networkConfig,
			})
			if network == nil || network.PublicNetworkEnable != tc.wantPublic ||
				network.PrivateNetworkEnable != tc.wantPrivate ||
				network.SharedInternetAccessEnable != tc.wantShared {
				t.Fatalf("network config = %#v, want public=%t private=%t shared=%t", network, tc.wantPublic, tc.wantPrivate, tc.wantShared)
			}
		})
	}
}

func TestTemplateUpdateRequestFromDiffSendsKecInstanceSpecChange(t *testing.T) {
	old := templateWithKS3("old description", "/mnt/old")
	old.Spec.Template.Spec.KecConfig = &sandboxv1.RuntimeKecConfig{
		InstanceSpecs: []sandboxv1.KecInstanceSpec{{
			InstanceType: "S6.2B",
			SystemDisk:   &sandboxv1.SystemDiskSpec{Type: "SSD3.0", Size: resource.MustParse("20Gi")},
		}},
	}
	next := templateWithKS3("old description", "/mnt/old")
	next.Spec.Template.Spec.KecConfig = &sandboxv1.RuntimeKecConfig{
		InstanceSpecs: []sandboxv1.KecInstanceSpec{{
			InstanceType: "S6.2A",
			SystemDisk:   &sandboxv1.SystemDiskSpec{Type: "SSD3.0", Size: resource.MustParse("20Gi")},
		}},
	}

	req := TemplateUpdateRequestFromDiff(next, old, RuntimeCredentials{})
	if req.KecConfig == nil || len(req.KecConfig.InstanceSpecs) != 1 || req.KecConfig.InstanceSpecs[0].InstanceType != "S6.2A" {
		t.Fatalf("kec instance spec diff should send KecConfig.InstanceSpecs: %#v", req.KecConfig)
	}
}

func TestTemplatePoolAndObservabilityLiveUnderRuntimeSpec(t *testing.T) {
	obj := templateWithKS3("description", "/mnt/data")
	obj.Spec.Access = "Private"
	obj.Spec.Template.Spec.Pool = &sandboxv1.TemplatePoolSpec{TargetSize: 2}
	obj.Spec.Template.Spec.Observability = &sandboxv1.ObservabilitySpec{
		Logging: &sandboxv1.LoggingSpec{Enabled: true},
	}

	createReq := TemplateCreateRequest(obj, RuntimeCredentials{})
	if createReq.PreheatConfig == nil || !createReq.PreheatConfig.PreheatEnable || createReq.PreheatConfig.PreheatNumber != 2 {
		t.Fatalf("nested pool was not mapped to preheat config: %#v", createReq.PreheatConfig)
	}
	if createReq.KlogConfig == nil || !createReq.KlogConfig.Enabled {
		t.Fatalf("nested observability was not mapped to klog config: %#v", createReq.KlogConfig)
	}

	var synced sandboxv1.SandboxTemplate
	ApplyTemplateSpecFromOpenAPI(&synced, openapi.Template{
		TemplateType:     "custom",
		TemplateCategory: "Private",
		KecConfig: &openapi.KecConfig{
			Enabled: true,
			InstanceSpecs: []openapi.InstanceSpec{{
				InstanceType: "N3.2B",
				SystemDisk:   &openapi.SystemDisk{Type: "ESSD_PL0", SizeGB: 80},
			}},
		},
		PreheatConfig: &openapi.PreheatConfig{PreheatEnable: true, PreheatNumber: 3},
		KlogConfig:    &openapi.KlogConfig{Enabled: true, ProjectName: "project", PoolName: "pool"},
	})
	kec := synced.Spec.Template.Spec.KecConfig
	if kec == nil || len(kec.InstanceSpecs) != 1 || kec.InstanceSpecs[0].InstanceType != "N3.2B" ||
		kec.InstanceSpecs[0].SystemDisk == nil || kec.InstanceSpecs[0].SystemDisk.Type != "ESSD_PL0" {
		t.Fatalf("remote kec was not written under template.spec.kecConfig: %#v", synced.Spec.Template.Spec.KecConfig)
	}
	if kec.InstanceSpecs[0].SystemDisk == nil || kec.InstanceSpecs[0].SystemDisk.Size.String() != "80Gi" {
		t.Fatalf("remote system disk was not written under template.spec.kecConfig.systemDisk.size: %#v", synced.Spec.Template.Spec.KecConfig)
	}
	if synced.Spec.Template == nil || synced.Spec.Template.Spec.Pool == nil || synced.Spec.Template.Spec.Pool.TargetSize != 3 {
		t.Fatalf("remote preheat was not written under template.spec.pool: %#v", synced.Spec.Template)
	}
	logging := synced.Spec.Template.Spec.Observability.Logging
	if logging == nil || !logging.Enabled || logging.ProjectName != "project" || logging.ContainerPoolName != "pool" {
		t.Fatalf("remote klog was not written under template.spec.observability: %#v", synced.Spec.Template.Spec.Observability)
	}

	synced.Status.Klog = &sandboxv1.KlogStatus{ProjectName: "old", PoolName: "old"}
	ApplyTemplateStatusFromOpenAPI(&synced, openapi.Template{Status: "Ready"})
	if synced.Status.Klog != nil {
		t.Fatalf("status.klog should be cleared during sync: %#v", synced.Status.Klog)
	}
}

func TestApplyTemplateSpecFromOpenAPIWritesKecInstanceSpecs(t *testing.T) {
	var synced sandboxv1.SandboxTemplate
	ApplyTemplateSpecFromOpenAPI(&synced, openapi.Template{
		TemplateType:     "custom",
		TemplateCategory: "Private",
		KecConfig: &openapi.KecConfig{
			Enabled: true,
			InstanceSpecs: []openapi.InstanceSpec{{
				InstanceType: "S6.2B",
				CPU:          2,
				Memory:       4,
				SystemDisk:   &openapi.SystemDisk{Type: "SSD3.0", SizeGB: 20},
				DataDisks: []openapi.DataDisk{{
					Type:               "SSD3.0",
					SizeGB:             50,
					DeleteWithInstance: true,
					Path:               "/data",
					FsType:             "ext4",
				}},
			}},
		},
	})

	kec := synced.Spec.Template.Spec.KecConfig
	if kec == nil || len(kec.InstanceSpecs) != 1 {
		t.Fatalf("remote instance specs were not written under template.spec.kecConfig: %#v", kec)
	}
	if kec.CPU != "2" || kec.Memory.String() != "4Gi" {
		t.Fatalf("remote global cpu/memory mismatch: %#v", kec)
	}
	spec := kec.InstanceSpecs[0]
	if spec.InstanceType != "S6.2B" {
		t.Fatalf("remote instance spec basic fields mismatch: %#v", spec)
	}
	if spec.SystemDisk == nil || spec.SystemDisk.Type != "SSD3.0" || spec.SystemDisk.Size.String() != "20Gi" {
		t.Fatalf("remote instance spec system disk mismatch: %#v", spec.SystemDisk)
	}
	if len(spec.DataDisks) != 1 || spec.DataDisks[0].Size.String() != "50Gi" || spec.DataDisks[0].FsType != "ext4" {
		t.Fatalf("remote instance spec data disks mismatch: %#v", spec.DataDisks)
	}
}

func TestPublicTemplateDoesNotUsePoolOrPreheat(t *testing.T) {
	obj := templateWithKS3("description", "/mnt/data")
	obj.Spec.Access = "Public"
	obj.Spec.Template.Spec.Pool = &sandboxv1.TemplatePoolSpec{TargetSize: 2}

	createReq := TemplateCreateRequest(obj, RuntimeCredentials{})
	if createReq.PreheatConfig != nil {
		t.Fatalf("public template create should not include preheat config: %#v", createReq.PreheatConfig)
	}

	old := templateWithKS3("description", "/mnt/data")
	old.Spec.Access = "Public"
	next := templateWithKS3("description", "/mnt/data")
	next.Spec.Access = "Public"
	next.Spec.Template.Spec.Pool = &sandboxv1.TemplatePoolSpec{TargetSize: 3}
	updateReq := TemplateUpdateRequestFromDiff(next, old, RuntimeCredentials{})
	if updateReq.PreheatConfig != nil {
		t.Fatalf("public template update should not include preheat config: %#v", updateReq.PreheatConfig)
	}

	var synced sandboxv1.SandboxTemplate
	ApplyTemplateSpecFromOpenAPI(&synced, openapi.Template{
		TemplateCategory: "Public",
		PreheatConfig:    &openapi.PreheatConfig{PreheatEnable: true, PreheatNumber: 3, PreheatedInstanceNumber: 2},
	})
	if synced.Spec.Template != nil && synced.Spec.Template.Spec.Pool != nil {
		t.Fatalf("public template sync should not write spec pool: %#v", synced.Spec.Template.Spec.Pool)
	}
	ApplyTemplateStatusFromOpenAPI(&synced, openapi.Template{
		TemplateCategory: "Public",
		PreheatConfig:    &openapi.PreheatConfig{PreheatEnable: true, PreheatNumber: 3, PreheatedInstanceNumber: 2},
	})
	if synced.Status.Preheat != nil {
		t.Fatalf("public template sync should not write status preheat: %#v", synced.Status.Preheat)
	}
}

func TestApplySandboxSpecDoesNotWriteSandboxName(t *testing.T) {
	obj := &sandboxv1.Sandbox{}
	ApplySandboxSpecFromOpenAPI(obj, openapi.Sandbox{SandboxName: "remote-name"})
	if obj.Spec.Name != "" {
		t.Fatalf("sandbox name should not be written to spec, got %q", obj.Spec.Name)
	}
}

func TestApplySandboxStatusIncludesRuntimeDetails(t *testing.T) {
	obj := &sandboxv1.Sandbox{
		Status: sandboxv1.SandboxStatus{
			CustomConfiguration: &sandboxv1.SandboxCustomConfiguration{ImageURL: "old"},
		},
	}
	ApplySandboxStatusFromOpenAPI(obj, openapi.Sandbox{
		Domain:   "https://domain.example.com",
		Endpoint: "https://endpoint.example.com",
		URLs: &openapi.URLs{
			Code:        "https://code.example.com",
			TerminalURL: "https://terminal.example.com",
		},
		AccessURL: &openapi.URLs{
			Code: "https://access-url.example.com",
		},
		SdnsURLs: map[string]string{"app": "https://sdns.example.com"},
		CustomConfiguration: &openapi.CustomConfiguration{
			ImageURL: "hub.kce.ksyun.com/sandbox/aio:v1",
			Port:     8000,
			Command:  "/entrypoint.sh",
		},
		Envs: []openapi.Env{{Key: "APP_ENV", Value: "prod"}},
		KS3MountConfig: &openapi.MountConfig{
			EnableKS3: true,
			MountPoints: []openapi.MountPoint{{
				BucketName:     "bucket-a",
				RemotePath:     "/datasets",
				LocalMountPath: "/mnt/ks3",
				ReadOnly:       true,
			}},
		},
		KPFSMountConfig: &openapi.MountConfig{
			EnableKPFS: true,
			KPFSMounts: []openapi.MountPoint{{
				FileSystemName: "fs-a",
				RemotePath:     "/",
				LocalMountPath: "/mnt/kpfs",
				ReadOnly:       false,
			}},
		},
	})

	if obj.Status.Endpoint != "https://endpoint.example.com" {
		t.Fatalf("sandbox endpoint mismatch: %q", obj.Status.Endpoint)
	}
	if obj.Status.URLs == nil || obj.Status.URLs.CodeURL != "https://code.example.com" || obj.Status.URLs.TerminalURL != "https://terminal.example.com" {
		t.Fatalf("sandbox urls were not synced to status: %#v", obj.Status.URLs)
	}
	if obj.Status.AccessURL == nil || obj.Status.AccessURL.CodeURL != "https://access-url.example.com" {
		t.Fatalf("sandbox accessUrl was not synced to status: %#v", obj.Status.AccessURL)
	}
	if obj.Status.SdnsURLs["app"] != "https://sdns.example.com" {
		t.Fatalf("sandbox sdnsUrls were not synced to status: %#v", obj.Status.SdnsURLs)
	}
	if obj.Status.ImageURL != "hub.kce.ksyun.com/sandbox/aio:v1" || obj.Status.Port != 8000 || obj.Status.Command != "/entrypoint.sh" {
		t.Fatalf("sandbox runtime config was not split into status fields: %#v", obj.Status)
	}
	if obj.Status.CustomConfiguration != nil {
		t.Fatalf("status.customConfiguration should be cleared during sync: %#v", obj.Status.CustomConfiguration)
	}
	if len(obj.Status.Env) != 1 || obj.Status.Env[0].Key != "APP_ENV" || obj.Status.Env[0].Value != "prod" {
		t.Fatalf("sandbox env was not synced to status: %#v", obj.Status.Env)
	}
	if obj.Status.Ks3MountConfig == nil || len(obj.Status.Ks3MountConfig.MountPoints) != 1 || obj.Status.Ks3MountConfig.MountPoints[0].BucketName != "bucket-a" {
		t.Fatalf("sandbox KS3 mount config mismatch: %#v", obj.Status.Ks3MountConfig)
	}
	if obj.Status.KpfsMountConfig == nil || len(obj.Status.KpfsMountConfig.MountPoints) != 1 || obj.Status.KpfsMountConfig.MountPoints[0].FileSystemName != "fs-a" {
		t.Fatalf("sandbox KPFS mount config mismatch: %#v", obj.Status.KpfsMountConfig)
	}
}

func TestApplySandboxStatusPreservesAccessURLWhenDetailOmitsIt(t *testing.T) {
	obj := &sandboxv1.Sandbox{
		Status: sandboxv1.SandboxStatus{
			AccessURL: &sandboxv1.SandboxURLs{CodeURL: "https://access-url.example.com"},
		},
	}

	ApplySandboxStatusFromOpenAPI(obj, openapi.Sandbox{Status: "RUNNING"})

	if obj.Status.AccessURL == nil || obj.Status.AccessURL.CodeURL != "https://access-url.example.com" {
		t.Fatalf("sandbox accessUrl should be preserved when openapi detail omits it: %#v", obj.Status.AccessURL)
	}
}

func TestApplySandboxAccessURLFromOpenAPI(t *testing.T) {
	obj := &sandboxv1.Sandbox{}

	ApplySandboxAccessURLFromOpenAPI(obj, openapi.Sandbox{
		AccessURL: &openapi.URLs{Code: "https://access-url.example.com"},
	})

	if obj.Status.AccessURL == nil || obj.Status.AccessURL.CodeURL != "https://access-url.example.com" {
		t.Fatalf("sandbox accessUrl was not applied from list response: %#v", obj.Status.AccessURL)
	}
}

func TestSandboxInlineTemplateObject(t *testing.T) {
	obj := &sandboxv1.Sandbox{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-a",
			Name:      "sandbox-a",
		},
		Spec: sandboxv1.SandboxSpec{
			OpenAPICredentialRef: &sandboxv1.OpenAPICredentialReference{Name: "openapi-cred"},
			Template: &sandboxv1.SandboxInlineTemplate{
				Description: "inline description",
				Type:        "Custom",
				Access:      "Private",
				Spec: sandboxv1.RuntimeTemplateSpec{
					StartCommand: "/start.sh",
				},
			},
		},
	}

	tpl := SandboxInlineTemplateObject(obj)
	if tpl == nil {
		t.Fatalf("inline template object should be built")
	}
	if tpl.Namespace != "ns-a" || tpl.Name != "sandbox-a-inline-template" {
		t.Fatalf("inline template metadata mismatch: %s/%s", tpl.Namespace, tpl.Name)
	}
	if tpl.Spec.OpenAPICredentialRef == nil || tpl.Spec.OpenAPICredentialRef.Name != "openapi-cred" {
		t.Fatalf("inline template should inherit openapi credential ref: %#v", tpl.Spec.OpenAPICredentialRef)
	}
	if tpl.Spec.Template == nil || tpl.Spec.Template.Spec.StartCommand != "/start.sh" {
		t.Fatalf("inline runtime template was not copied: %#v", tpl.Spec.Template)
	}
}

func TestApplySandboxSpecPreservesInlineTemplateSource(t *testing.T) {
	obj := &sandboxv1.Sandbox{
		Spec: sandboxv1.SandboxSpec{
			Template: &sandboxv1.SandboxInlineTemplate{Type: "Custom", Access: "Private"},
		},
	}

	ApplySandboxSpecFromOpenAPI(obj, openapi.Sandbox{TemplateID: "tpl-inline"})
	if obj.Spec.TemplateRef.ID != "" {
		t.Fatalf("inline sandbox should not be rewritten to templateRef: %#v", obj.Spec.TemplateRef)
	}
}

func templateWithKS3(description, mountPath string) *sandboxv1.SandboxTemplate {
	memory := resource.MustParse("4Gi")
	return &sandboxv1.SandboxTemplate{
		Spec: sandboxv1.SandboxTemplateSpec{
			Description: description,
			Access:      "Private",
			Type:        "CUSTOM",
			Template: &sandboxv1.RuntimeTemplate{Spec: sandboxv1.RuntimeTemplateSpec{
				KecConfig: &sandboxv1.RuntimeKecConfig{CPU: "2", Memory: memory},
				StorageCredentialRef: &sandboxv1.LocalObjectReference{
					Name: "ks3-credential",
				},
				Ks3MountConfig: &sandboxv1.MountConfig{
					Enabled: true,
					MountPoints: []sandboxv1.MountPoint{{
						BucketName:     "bucket-a",
						RemotePath:     "/datasets",
						LocalMountPath: mountPath,
					}},
				},
			}},
		},
	}
}

package mapper

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

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
		KS3ByName: map[string]*credentials.RuntimeCredential{
			"ks3-credential": {AccessKey: "ak", SecretAccessKey: "sk"},
		},
	})
	if req.KS3MountConfig == nil || !req.KS3MountConfig.EnableKS3 {
		t.Fatalf("changed mount config should be included: %#v", req)
	}
	if req.AccessKey != "ak" || req.SecretAccessKey != "sk" {
		t.Fatalf("mount credential should be included, got ak=%q sk=%q", req.AccessKey, req.SecretAccessKey)
	}

	deletedMount := templateWithKS3("old description", "/mnt/old")
	deletedMount.Spec.Template.Spec.Volumes = nil
	req = TemplateUpdateRequestFromDiff(deletedMount, old, RuntimeCredentials{})
	if req.KS3MountConfig == nil || req.KS3MountConfig.EnableKS3 {
		t.Fatalf("deleted KS3 mount must send Ks3Enable=false: %#v", req.KS3MountConfig)
	}
	if req.KPFSMountConfig == nil || req.KPFSMountConfig.EnableKPFS {
		t.Fatalf("mount diff must include KpfsEnable=false target state: %#v", req.KPFSMountConfig)
	}
}

func TestTemplateUpdateRequestFromDiffSendsDiskAsKecConfig(t *testing.T) {
	old := templateWithKS3("old description", "/mnt/old")
	next := templateWithKS3("old description", "/mnt/old")
	next.Spec.Template.Spec.Resources.Disk = resource.MustParse("80Gi")

	req := TemplateUpdateRequestFromDiff(next, old, RuntimeCredentials{})
	if req.KecConfig == nil || !req.KecConfig.Enabled || req.KecConfig.SystemDiskSizeGB != 80 {
		t.Fatalf("disk diff should send KecConfig with SystemDiskSize=80Gi, got %#v", req.KecConfig)
	}

	err := CompleteTemplateUpdateRequestFromRemote(&req, openapi.Template{KecConfig: &openapi.KecConfig{
		InstanceType:     "N3.2B",
		SystemDiskType:   "ESSD_PL0",
		SystemDiskSizeGB: 40,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if req.KecConfig.InstanceType != "N3.2B" || req.KecConfig.SystemDiskType != "ESSD_PL0" || req.KecConfig.SystemDiskSizeGB != 80 {
		t.Fatalf("remote KecConfig completion failed: %#v", req.KecConfig)
	}
}

func TestTemplatePoolAndObservabilityLiveUnderRuntimeSpec(t *testing.T) {
	obj := templateWithKS3("description", "/mnt/data")
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
		PreheatConfig:    &openapi.PreheatConfig{PreheatEnable: true, PreheatNumber: 3},
		KlogConfig:       &openapi.KlogConfig{Enabled: true, ProjectName: "project", PoolName: "pool"},
	})
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

func TestApplySandboxSpecPreservesLocalSandboxName(t *testing.T) {
	obj := &sandboxv1.Sandbox{}
	ApplySandboxSpecFromOpenAPI(obj, openapi.Sandbox{SandboxName: "remote-name"})
	if obj.Spec.Name != "remote-name" {
		t.Fatalf("empty local sandbox name should use remote name, got %q", obj.Spec.Name)
	}

	obj.Spec.Name = "user-name"
	ApplySandboxSpecFromOpenAPI(obj, openapi.Sandbox{SandboxName: "remote-name-2"})
	if obj.Spec.Name != "user-name" {
		t.Fatalf("local sandbox name should be preserved, got %q", obj.Spec.Name)
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
	if len(obj.Status.Volumes) != 2 {
		t.Fatalf("sandbox volumes were not synced to status: %#v", obj.Status.Volumes)
	}
	if obj.Status.Volumes[0].KS3 == nil || obj.Status.Volumes[0].KS3.Bucket != "bucket-a" {
		t.Fatalf("sandbox KS3 volume mismatch: %#v", obj.Status.Volumes[0])
	}
	if obj.Status.Volumes[1].KPFS == nil || obj.Status.Volumes[1].KPFS.FileSystem != "fs-a" {
		t.Fatalf("sandbox KPFS volume mismatch: %#v", obj.Status.Volumes[1])
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

func templateWithKS3(description, mountPath string) *sandboxv1.SandboxTemplate {
	memory := resource.MustParse("4Gi")
	return &sandboxv1.SandboxTemplate{
		Spec: sandboxv1.SandboxTemplateSpec{
			Description: description,
			Access:      "Private",
			Type:        "CUSTOM",
			Template: &sandboxv1.RuntimeTemplate{Spec: sandboxv1.RuntimeTemplateSpec{
				Resources: &sandboxv1.RuntimeResourceSpec{CPU: "2", Memory: memory},
				Volumes: []sandboxv1.TemplateVolume{{
					Name:      "ks3-data",
					Type:      "KS3",
					MountPath: mountPath,
					KS3: &sandboxv1.KS3VolumeSource{
						Bucket: "bucket-a",
						Path:   "/datasets",
						CredentialRef: &sandboxv1.LocalObjectReference{
							Name: "ks3-credential",
						},
					},
				}},
			}},
		},
	}
}

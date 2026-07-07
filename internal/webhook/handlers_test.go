package webhook

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sandboxv1 "sandbox-operator/api/v1alpha1"
)

func TestSandboxSpecOnlyTimeoutChanged(t *testing.T) {
	oldSpec := sandboxv1.SandboxSpec{
		Name:           "sandbox-a",
		TemplateRef:    sandboxv1.TemplateReference{ID: "tpl-1"},
		TimeoutSeconds: 3600,
		Env:            []sandboxv1.EnvVar{{Key: "APP_ENV", Value: "prod"}},
	}

	timeoutChanged := oldSpec
	timeoutChanged.TimeoutSeconds = 7200
	if !sandboxSpecOnlyTimeoutChanged(oldSpec, timeoutChanged) {
		t.Fatalf("timeout changes should be allowed")
	}

	nameChanged := oldSpec
	nameChanged.Name = "sandbox-b"
	if sandboxSpecOnlyTimeoutChanged(oldSpec, nameChanged) {
		t.Fatalf("name changes should be rejected")
	}

	envChanged := oldSpec
	envChanged.Env = []sandboxv1.EnvVar{{Key: "APP_ENV", Value: "dev"}}
	if sandboxSpecOnlyTimeoutChanged(oldSpec, envChanged) {
		t.Fatalf("env changes should be rejected")
	}

	templateChanged := oldSpec
	templateChanged.TemplateRef.ID = "tpl-2"
	if sandboxSpecOnlyTimeoutChanged(oldSpec, templateChanged) {
		t.Fatalf("templateRef changes should be rejected")
	}
}

func TestValidateTemplateRequiresCompleteKecConfig(t *testing.T) {
	h := &Handler{}
	obj := validTemplateForWebhook()
	obj.Spec.Template.Spec.Resources.Disk = resource.MustParse("80Gi")

	if err := h.validateTemplate(obj); err == nil {
		t.Fatalf("disk without kec instanceType/systemDiskType should be rejected")
	}

	obj.Spec.Template.Spec.Kec = &sandboxv1.KecSpec{
		InstanceType:   "N3.2B",
		SystemDiskType: "ESSD_PL0",
	}
	if err := h.validateTemplate(obj); err != nil {
		t.Fatalf("complete kec config should be accepted: %v", err)
	}
}

func TestValidateTemplateRejectsPublicPool(t *testing.T) {
	h := &Handler{}
	obj := validTemplateForWebhook()
	obj.Spec.Access = "Public"
	obj.Spec.Template.Spec.Pool = &sandboxv1.TemplatePoolSpec{TargetSize: 1}

	if err := h.validateTemplate(obj); err == nil {
		t.Fatalf("public template with pool should be rejected")
	}
}

func TestValidateSandboxTemplateSource(t *testing.T) {
	h := &Handler{}
	obj := &sandboxv1.Sandbox{}
	if err := h.validateSandboxTemplateSource(obj); err == nil {
		t.Fatalf("missing templateRef and inline template should be rejected")
	}

	obj.Spec.TemplateRef = sandboxv1.TemplateReference{ID: "tpl-1"}
	if err := h.validateSandboxTemplateSource(obj); err != nil {
		t.Fatalf("templateRef should be accepted: %v", err)
	}

	obj.Spec.TemplateRef = sandboxv1.TemplateReference{ID: "tpl-1", Name: "template-a"}
	if err := h.validateSandboxTemplateSource(obj); err != nil {
		t.Fatalf("templateRef source should be accepted before id/name resolution: %v", err)
	}
	if _, err := h.resolveTemplateID(nil, obj); err == nil {
		t.Fatalf("templateRef id and name together should be rejected during resolution")
	}

	obj.Spec.TemplateRef = sandboxv1.TemplateReference{ID: "tpl-1"}
	obj.Spec.Template = &sandboxv1.SandboxInlineTemplate{Type: "Custom", Access: "Private"}
	if err := h.validateSandboxTemplateSource(obj); err == nil {
		t.Fatalf("templateRef and inline template together should be rejected")
	}

	obj.Spec.TemplateRef = sandboxv1.TemplateReference{}
	if err := h.validateSandboxTemplateSource(obj); err != nil {
		t.Fatalf("inline template should be accepted: %v", err)
	}

	obj.Spec.Template.Type = ""
	if err := h.validateSandboxTemplateSource(obj); err == nil {
		t.Fatalf("inline template without type should be rejected")
	}
}

func validTemplateForWebhook() *sandboxv1.SandboxTemplate {
	return &sandboxv1.SandboxTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "test-template"},
		Spec: sandboxv1.SandboxTemplateSpec{
			Access: "Private",
			Type:   "Custom",
			Template: &sandboxv1.RuntimeTemplate{
				Spec: sandboxv1.RuntimeTemplateSpec{
					Resources: &sandboxv1.RuntimeResourceSpec{},
				},
			},
		},
	}
}

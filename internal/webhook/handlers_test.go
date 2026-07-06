package webhook

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sandboxv1 "sandbox-operator/api/v1alpha1"
)

func TestSandboxSpecOnlyNameOrTimeoutChanged(t *testing.T) {
	oldSpec := sandboxv1.SandboxSpec{
		Name:           "sandbox-a",
		TemplateRef:    sandboxv1.TemplateReference{ID: "tpl-1"},
		TimeoutSeconds: 3600,
		Env:            []sandboxv1.EnvVar{{Key: "APP_ENV", Value: "prod"}},
	}

	nameAndTimeout := oldSpec
	nameAndTimeout.Name = "sandbox-b"
	nameAndTimeout.TimeoutSeconds = 7200
	if !sandboxSpecOnlyNameOrTimeoutChanged(oldSpec, nameAndTimeout) {
		t.Fatalf("name and timeout changes should be allowed")
	}

	envChanged := oldSpec
	envChanged.Env = []sandboxv1.EnvVar{{Key: "APP_ENV", Value: "dev"}}
	if sandboxSpecOnlyNameOrTimeoutChanged(oldSpec, envChanged) {
		t.Fatalf("env changes should be rejected")
	}

	templateChanged := oldSpec
	templateChanged.TemplateRef.ID = "tpl-2"
	if sandboxSpecOnlyNameOrTimeoutChanged(oldSpec, templateChanged) {
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

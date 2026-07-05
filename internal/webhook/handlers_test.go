package webhook

import (
	"testing"

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

package v1alpha1

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSandboxTemplateStatusSerializesCanDeleteFalse(t *testing.T) {
	raw, err := json.Marshal(SandboxTemplateStatus{CanDelete: false})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"canDelete":false`) {
		t.Fatalf("canDelete=false must be explicit in status JSON, got %s", string(raw))
	}
}

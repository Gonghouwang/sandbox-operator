package annotations

import "encoding/json"

const (
	Prefix = "sandbox.kce.ksyun.com/"

	TemplateID     = Prefix + "template-id"
	SandboxID      = Prefix + "sandbox-id"
	SandboxIDs     = Prefix + "sandbox-ids"
	InlineTemplate = Prefix + "inline-template"
	Endpoint       = Prefix + "endpoint"
	Token          = Prefix + "token"
)

var ReservedKeys = []string{
	TemplateID,
	SandboxID,
	SandboxIDs,
	InlineTemplate,
	Endpoint,
	Token,
}

func HasReserved(values map[string]string) (string, bool) {
	for _, key := range ReservedKeys {
		if _, ok := values[key]; ok {
			return key, true
		}
	}
	return "", false
}

func ReservedChanged(oldValues, newValues map[string]string) (string, bool) {
	for _, key := range ReservedKeys {
		if oldValues[key] != newValues[key] {
			return key, true
		}
	}
	return "", false
}

func Set(values map[string]string, key, value string) map[string]string {
	if value == "" {
		return values
	}
	if values == nil {
		values = map[string]string{}
	}
	values[key] = value
	return values
}

func Get(values map[string]string, key string) string {
	if values == nil {
		return ""
	}
	return values[key]
}

func EncodeStringSlice(values []string) string {
	if len(values) == 0 {
		return ""
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(raw)
}

func DecodeStringSlice(value string) []string {
	if value == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil
	}
	return out
}

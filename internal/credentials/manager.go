package credentials

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sandboxv1 "sandbox-operator/api/v1alpha1"
)

const (
	DefaultOpenAPISecretName = "sandbox-openapi-credentials"

	KeyAccessKeyID     = "accessKeyId"
	KeySecretAccessKey = "secretAccessKey"
	KeyAccountID       = "accountId"
	KeyRegion          = "region"

	KeyRuntimeAccessKey       = "accessKey"
	KeyRuntimeSecretAccessKey = "secretAccessKey"
)

type OpenAPICredential struct {
	AccessKeyID     string
	SecretAccessKey string
	AccountID       string
	Region          string
	SecretName      string
}

type RuntimeCredential struct {
	AccessKey       string
	SecretAccessKey string
	SecretName      string
}

type RegistryCredential struct {
	Server     string
	Username   string
	Password   string
	SecretName string
}

type OpenAPICredentialNotFoundError struct {
	Namespace string
	Name      string
}

func (e *OpenAPICredentialNotFoundError) Error() string {
	return fmt.Sprintf("openapi credential secret %s/%s not found", e.Namespace, e.Name)
}

func IsOpenAPICredentialNotFound(err error) bool {
	var target *OpenAPICredentialNotFoundError
	return errors.As(err, &target)
}

type Manager struct {
	client                   client.Client
	defaultOpenAPISecretName string
}

func NewManager(c client.Client, defaultOpenAPISecretName string) *Manager {
	if defaultOpenAPISecretName == "" {
		defaultOpenAPISecretName = DefaultOpenAPISecretName
	}
	return &Manager{client: c, defaultOpenAPISecretName: defaultOpenAPISecretName}
}

func (m *Manager) GetOpenAPI(ctx context.Context, namespace string, ref *sandboxv1.OpenAPICredentialReference) (*OpenAPICredential, error) {
	name := m.defaultOpenAPISecretName
	if ref != nil && ref.Name != "" {
		name = ref.Name
	}

	var secret corev1.Secret
	if err := m.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, &OpenAPICredentialNotFoundError{Namespace: namespace, Name: name}
		}
		return nil, err
	}

	cred := &OpenAPICredential{
		AccessKeyID:     string(secret.Data[KeyAccessKeyID]),
		SecretAccessKey: string(secret.Data[KeySecretAccessKey]),
		AccountID:       string(secret.Data[KeyAccountID]),
		Region:          string(secret.Data[KeyRegion]),
		SecretName:      name,
	}
	if cred.AccessKeyID == "" || cred.SecretAccessKey == "" || cred.Region == "" {
		return nil, fmt.Errorf("openapi credential secret %s/%s is missing required keys", namespace, name)
	}
	return cred, nil
}

func (m *Manager) GetRuntime(ctx context.Context, namespace string, ref *sandboxv1.LocalObjectReference) (*RuntimeCredential, error) {
	if ref == nil || ref.Name == "" {
		return nil, nil
	}

	var secret corev1.Secret
	if err := m.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.Name}, &secret); err != nil {
		return nil, err
	}

	return &RuntimeCredential{
		AccessKey:       string(secret.Data[KeyRuntimeAccessKey]),
		SecretAccessKey: string(secret.Data[KeyRuntimeSecretAccessKey]),
		SecretName:      ref.Name,
	}, nil
}

func (m *Manager) GetRegistry(ctx context.Context, namespace string, ref *sandboxv1.RegistryCredentialReference) (*RegistryCredential, error) {
	if ref == nil || ref.Name == "" {
		return nil, nil
	}
	var secret corev1.Secret
	if err := m.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.Name}, &secret); err != nil {
		return nil, err
	}
	server := ref.Server
	username := string(secret.Data["username"])
	password := string(secret.Data["password"])
	if raw := secret.Data[corev1.DockerConfigJsonKey]; len(raw) > 0 {
		u, p, s := dockerConfigCredential(raw, server)
		if username == "" {
			username = u
		}
		if password == "" {
			password = p
		}
		if server == "" {
			server = s
		}
	}
	return &RegistryCredential{Server: server, Username: username, Password: password, SecretName: ref.Name}, nil
}

func dockerConfigCredential(raw []byte, preferredServer string) (string, string, string) {
	var cfg struct {
		Auths map[string]struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Auth     string `json:"auth"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", "", ""
	}
	for server, auth := range cfg.Auths {
		if preferredServer != "" && server != preferredServer {
			continue
		}
		username, password := auth.Username, auth.Password
		if (username == "" || password == "") && auth.Auth != "" {
			decoded, err := base64.StdEncoding.DecodeString(auth.Auth)
			if err == nil {
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) == 2 {
					username = parts[0]
					password = parts[1]
				}
			}
		}
		return username, password, server
	}
	return "", "", ""
}

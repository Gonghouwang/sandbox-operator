package credentials

import (
	"context"
	"fmt"

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
	KeyRuntimeToken           = "token"
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
	Token           string
	SecretName      string
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
			return nil, fmt.Errorf("openapi credential secret %s/%s not found", namespace, name)
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
		Token:           string(secret.Data[KeyRuntimeToken]),
		SecretName:      ref.Name,
	}, nil
}

package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"

	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	sandboxv1 "sandbox-operator/api/v1alpha1"
	"sandbox-operator/internal/credentials"
	"sandbox-operator/internal/mapper"
	"sandbox-operator/internal/openapi"
	"sandbox-operator/internal/operation"
)

const DefaultOperatorUsername = "system:serviceaccount:sandbox-operator-system:sandbox-operator"

type Handler struct {
	Client           client.Client
	Credentials      *credentials.Manager
	Operations       *operation.Recorder
	OpenAPI          openapi.Interface
	Decoder          admission.Decoder
	OperatorUsername string
	Kind             string
}

func NewHandler(c client.Client, scheme *runtime.Scheme, creds *credentials.Manager, ops *operation.Recorder, api openapi.Interface, kind string) *Handler {
	return &Handler{
		Client:           c,
		Credentials:      creds,
		Operations:       ops,
		OpenAPI:          api,
		Decoder:          admission.NewDecoder(scheme),
		OperatorUsername: DefaultOperatorUsername,
		Kind:             kind,
	}
}

func (h *Handler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if h.isOperator(req) {
		return admission.Allowed("operator request")
	}
	if req.DryRun != nil && *req.DryRun {
		return admission.Allowed("dry run")
	}

	switch h.Kind {
	case "SandboxTemplate":
		return h.handleTemplate(ctx, req)
	case "Sandbox":
		return h.handleSandbox(ctx, req)
	case "SandboxClaim":
		return h.handleClaim(ctx, req)
	default:
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("unsupported webhook kind %s", h.Kind))
	}
}

func (h *Handler) isOperator(req admission.Request) bool {
	username := h.OperatorUsername
	if username == "" {
		username = DefaultOperatorUsername
	}
	return req.UserInfo.Username == username
}

func (h *Handler) handleTemplate(ctx context.Context, req admission.Request) admission.Response {
	switch req.Operation {
	case admissionv1.Create:
		var obj sandboxv1.SandboxTemplate
		if err := h.Decoder.Decode(req, &obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := h.validateTemplate(&obj); err != nil {
			return admission.Denied(err.Error())
		}
		cred, runtimeCreds, err := h.templateCredentials(ctx, &obj)
		if err != nil {
			return admission.Denied(err.Error())
		}
		created, err := h.OpenAPI.CreateTemplate(ctx, mapper.OpenAPICredential(cred), mapper.TemplateCreateRequest(&obj, runtimeCreds))
		if err != nil {
			return admission.Denied(err.Error())
		}
		if h.Operations != nil {
			_ = h.Operations.Upsert(ctx, operation.Record{
				Namespace:  obj.Namespace,
				Kind:       "SandboxTemplate",
				Name:       obj.Name,
				Generation: strconv.FormatInt(obj.Generation, 10),
				Action:     "Create",
				TemplateID: created.TemplateID,
			})
		}
		return admission.Allowed("template created in openapi")
	case admissionv1.Update:
		var obj sandboxv1.SandboxTemplate
		var old sandboxv1.SandboxTemplate
		if err := h.Decoder.Decode(req, &obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := h.decodeOld(req, &old); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := h.validateTemplate(&obj); err != nil {
			return admission.Denied(err.Error())
		}
		if old.Spec.OpenAPICredentialRef != nil && obj.Spec.OpenAPICredentialRef != nil && old.Spec.OpenAPICredentialRef.Name != obj.Spec.OpenAPICredentialRef.Name {
			return admission.Denied("spec.openapiCredentialRef is immutable")
		}
		if reflect.DeepEqual(old.Spec, obj.Spec) {
			return admission.Allowed("template spec unchanged")
		}
		if obj.Status.TemplateID == "" {
			return admission.Denied("status.templateID is empty; wait for OpenAPI sync before updating")
		}
		cred, runtimeCreds, err := h.templateCredentials(ctx, &obj)
		if err != nil {
			return admission.Denied(err.Error())
		}
		if err := h.OpenAPI.UpdateTemplate(ctx, mapper.OpenAPICredential(cred), mapper.TemplateUpdateRequest(&obj, runtimeCreds)); err != nil {
			return admission.Denied(err.Error())
		}
		return admission.Allowed("template updated in openapi")
	case admissionv1.Delete:
		return admission.Allowed("template deletion handled by finalizer")
	default:
		return admission.Allowed("operation ignored")
	}
}

func (h *Handler) handleSandbox(ctx context.Context, req admission.Request) admission.Response {
	switch req.Operation {
	case admissionv1.Create:
		var obj sandboxv1.Sandbox
		if err := h.Decoder.Decode(req, &obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := h.validateSandboxName(ctx, &obj, ""); err != nil {
			return admission.Denied(err.Error())
		}
		templateID, err := h.resolveTemplateID(ctx, &obj)
		if err != nil {
			return admission.Denied(err.Error())
		}
		cred, runtimeCreds, err := h.sandboxCredentials(ctx, &obj)
		if err != nil {
			return admission.Denied(err.Error())
		}
		started, err := h.OpenAPI.StartSandbox(ctx, mapper.OpenAPICredential(cred), mapper.SandboxStartRequest(&obj, templateID, runtimeCreds))
		if err != nil {
			return admission.Denied(err.Error())
		}
		if h.Operations != nil {
			_ = h.Operations.Upsert(ctx, operation.Record{
				Namespace:  obj.Namespace,
				Kind:       "Sandbox",
				Name:       obj.Name,
				Generation: strconv.FormatInt(obj.Generation, 10),
				Action:     "Create",
				TemplateID: started.TemplateID,
				SandboxID:  started.SandboxID,
				Endpoint:   started.Endpoint,
				Token:      started.Token,
			})
		}
		return admission.Allowed("sandbox started in openapi")
	case admissionv1.Update:
		var obj sandboxv1.Sandbox
		var old sandboxv1.Sandbox
		if err := h.Decoder.Decode(req, &obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := h.decodeOld(req, &old); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := h.validateSandboxName(ctx, &obj, old.Name); err != nil {
			return admission.Denied(err.Error())
		}
		return admission.Allowed("sandbox update accepted; OpenAPI has no sandbox update action")
	case admissionv1.Delete:
		return admission.Allowed("sandbox deletion handled by finalizer")
	default:
		return admission.Allowed("operation ignored")
	}
}

func (h *Handler) handleClaim(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create {
		return admission.Allowed("claim update/delete deferred to controller")
	}
	var obj sandboxv1.SandboxClaim
	if err := h.Decoder.Decode(req, &obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if obj.Spec.Replicas < 1 {
		return admission.Denied("spec.replicas must be greater than zero")
	}
	templateID, err := h.resolveClaimTemplateID(ctx, &obj)
	if err != nil {
		return admission.Denied(err.Error())
	}
	cred, err := h.Credentials.GetOpenAPI(ctx, obj.Namespace, obj.Spec.OpenAPICredentialRef)
	if err != nil {
		return admission.Denied(err.Error())
	}
	sandboxIDs := make([]string, 0, obj.Spec.Replicas)
	for i := 0; i < obj.Spec.Replicas; i++ {
		name := fmt.Sprintf("%s-%d", obj.Name, i)
		if err := h.ensureSandboxNameAvailable(ctx, obj.Namespace, name, ""); err != nil {
			return admission.Denied(err.Error())
		}
		req := openapi.StartSandboxRequest{
			TemplateID: templateID,
			Timeout:    obj.Spec.TimeoutSeconds,
			EnvVars:    mapper.SandboxStartRequest(&sandboxv1.Sandbox{Spec: sandboxv1.SandboxSpec{Env: obj.Spec.Env}}, templateID, mapper.RuntimeCredentials{}).EnvVars,
		}
		started, err := h.OpenAPI.StartSandbox(ctx, mapper.OpenAPICredential(cred), req)
		if err != nil {
			return admission.Denied(err.Error())
		}
		sandboxIDs = append(sandboxIDs, started.SandboxID)
	}
	if h.Operations != nil {
		_ = h.Operations.Upsert(ctx, operation.Record{
			Namespace:  obj.Namespace,
			Kind:       "SandboxClaim",
			Name:       obj.Name,
			Generation: strconv.FormatInt(obj.Generation, 10),
			Action:     "Create",
			TemplateID: templateID,
			SandboxIDs: sandboxIDs,
		})
	}
	return admission.Allowed("claim sandboxes started in openapi")
}

func (h *Handler) templateCredentials(ctx context.Context, obj *sandboxv1.SandboxTemplate) (*credentials.OpenAPICredential, mapper.RuntimeCredentials, error) {
	cred, err := h.Credentials.GetOpenAPI(ctx, obj.Namespace, obj.Spec.OpenAPICredentialRef)
	if err != nil {
		return nil, mapper.RuntimeCredentials{}, err
	}
	var runtimeCreds mapper.RuntimeCredentials
	if obj.Spec.Template != nil {
		tpl := obj.Spec.Template.Spec
		if tpl.Image != nil && tpl.Image.RegistryCredentialRef != nil {
			runtimeCreds.Registry, err = h.Credentials.GetRegistry(ctx, obj.Namespace, tpl.Image.RegistryCredentialRef)
			if err != nil {
				return nil, mapper.RuntimeCredentials{}, err
			}
		}
		runtimeCreds.KS3ByName = map[string]*credentials.RuntimeCredential{}
		runtimeCreds.KPFSByName = map[string]*credentials.RuntimeCredential{}
		for _, volume := range tpl.Volumes {
			switch {
			case volume.KS3 != nil && volume.KS3.CredentialRef != nil:
				vc, err := h.Credentials.GetRuntime(ctx, obj.Namespace, volume.KS3.CredentialRef)
				if err != nil {
					return nil, mapper.RuntimeCredentials{}, err
				}
				runtimeCreds.KS3ByName[volume.KS3.CredentialRef.Name] = vc
			case volume.KPFS != nil && volume.KPFS.CredentialRef != nil:
				vc, err := h.Credentials.GetRuntime(ctx, obj.Namespace, volume.KPFS.CredentialRef)
				if err != nil {
					return nil, mapper.RuntimeCredentials{}, err
				}
				runtimeCreds.KPFSByName[volume.KPFS.CredentialRef.Name] = vc
			}
		}
	}
	if obj.Spec.Observability != nil && obj.Spec.Observability.Logging != nil {
		runtimeCreds.Klog, err = h.Credentials.GetRuntime(ctx, obj.Namespace, obj.Spec.Observability.Logging.CredentialRef)
		if err != nil {
			return nil, mapper.RuntimeCredentials{}, err
		}
	}
	return cred, runtimeCreds, nil
}

func (h *Handler) sandboxCredentials(ctx context.Context, obj *sandboxv1.Sandbox) (*credentials.OpenAPICredential, mapper.RuntimeCredentials, error) {
	cred, err := h.Credentials.GetOpenAPI(ctx, obj.Namespace, obj.Spec.OpenAPICredentialRef)
	if err != nil {
		return nil, mapper.RuntimeCredentials{}, err
	}
	var runtimeCreds mapper.RuntimeCredentials
	if obj.Spec.Ks3MountConfig != nil {
		runtimeCreds.KS3, err = h.Credentials.GetRuntime(ctx, obj.Namespace, obj.Spec.Ks3MountConfig.CredentialRef)
		if err != nil {
			return nil, mapper.RuntimeCredentials{}, err
		}
	}
	if obj.Spec.KpfsMountConfig != nil {
		runtimeCreds.KPFS, err = h.Credentials.GetRuntime(ctx, obj.Namespace, obj.Spec.KpfsMountConfig.CredentialRef)
		if err != nil {
			return nil, mapper.RuntimeCredentials{}, err
		}
	}
	return cred, runtimeCreds, nil
}

func (h *Handler) validateTemplate(obj *sandboxv1.SandboxTemplate) error {
	if obj.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if obj.Spec.Access == "" || obj.Spec.Type == "" {
		return fmt.Errorf("spec.access and spec.type are required")
	}
	return nil
}

func (h *Handler) resolveTemplateID(ctx context.Context, obj *sandboxv1.Sandbox) (string, error) {
	if obj.Spec.TemplateRef.ID != "" && obj.Spec.TemplateRef.Name != "" {
		return "", fmt.Errorf("spec.templateRef.id and spec.templateRef.name are mutually exclusive")
	}
	if obj.Spec.TemplateRef.ID != "" {
		return obj.Spec.TemplateRef.ID, nil
	}
	if obj.Spec.TemplateRef.Name == "" {
		return "", fmt.Errorf("spec.templateRef.id or spec.templateRef.name is required")
	}
	var template sandboxv1.SandboxTemplate
	if err := h.Client.Get(ctx, client.ObjectKey{Namespace: obj.Namespace, Name: obj.Spec.TemplateRef.Name}, &template); err != nil {
		return "", err
	}
	if template.Status.TemplateID == "" {
		return "", fmt.Errorf("referenced template %s has empty status.templateID", obj.Spec.TemplateRef.Name)
	}
	return template.Status.TemplateID, nil
}

func (h *Handler) resolveClaimTemplateID(ctx context.Context, obj *sandboxv1.SandboxClaim) (string, error) {
	if obj.Spec.TemplateRef.ID != "" {
		return obj.Spec.TemplateRef.ID, nil
	}
	if obj.Spec.TemplateRef.Name == "" {
		return "", fmt.Errorf("spec.templateRef.id or spec.templateRef.name is required")
	}
	var template sandboxv1.SandboxTemplate
	if err := h.Client.Get(ctx, client.ObjectKey{Namespace: obj.Namespace, Name: obj.Spec.TemplateRef.Name}, &template); err != nil {
		return "", err
	}
	if template.Status.TemplateID == "" {
		return "", fmt.Errorf("referenced template %s has empty status.templateID", obj.Spec.TemplateRef.Name)
	}
	return template.Status.TemplateID, nil
}

func (h *Handler) validateSandboxName(ctx context.Context, obj *sandboxv1.Sandbox, currentObjectName string) error {
	effectiveName := obj.Spec.Name
	if effectiveName == "" {
		effectiveName = obj.Name
	}
	return h.ensureSandboxNameAvailable(ctx, obj.Namespace, effectiveName, currentObjectName)
}

func (h *Handler) ensureSandboxNameAvailable(ctx context.Context, namespace, sandboxName, currentObjectName string) error {
	var list sandboxv1.SandboxList
	if err := h.Client.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return err
	}
	for _, item := range list.Items {
		if currentObjectName != "" && item.Name == currentObjectName {
			continue
		}
		effective := item.Spec.Name
		if effective == "" {
			effective = item.Name
		}
		if effective == sandboxName {
			return fmt.Errorf("sandbox name %q already exists in namespace %s", sandboxName, namespace)
		}
	}
	return nil
}

func (h *Handler) decodeOld(req admission.Request, into client.Object) error {
	if len(req.OldObject.Raw) == 0 {
		if req.Operation == admissionv1.Delete && len(req.Object.Raw) > 0 {
			return json.Unmarshal(req.Object.Raw, into)
		}
		return fmt.Errorf("old object is empty")
	}
	if err := h.Decoder.DecodeRaw(req.OldObject, into); err != nil {
		if apierrors.IsBadRequest(err) {
			return err
		}
		return err
	}
	return nil
}

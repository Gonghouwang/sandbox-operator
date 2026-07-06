package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	sandboxv1 "sandbox-operator/api/v1alpha1"
	"sandbox-operator/internal/annotations"
	"sandbox-operator/internal/credentials"
	"sandbox-operator/internal/mapper"
	"sandbox-operator/internal/openapi"
)

const DefaultOperatorUsername = "system:serviceaccount:sandbox-operator-system:sandbox-operator"

type Mode string

const (
	ModeValidate Mode = "validate"
	ModeMutate   Mode = "mutate"
)

type Handler struct {
	Client           client.Client
	Credentials      *credentials.Manager
	OpenAPI          openapi.Interface
	Decoder          admission.Decoder
	OperatorUsername string
	Kind             string
	Mode             Mode
}

func NewHandler(c client.Client, scheme *runtime.Scheme, creds *credentials.Manager, api openapi.Interface, kind string) *Handler {
	return &Handler{
		Client:           c,
		Credentials:      creds,
		OpenAPI:          api,
		Decoder:          admission.NewDecoder(scheme),
		OperatorUsername: DefaultOperatorUsername,
		Kind:             kind,
		Mode:             ModeValidate,
	}
}

func NewMutatingHandler(c client.Client, scheme *runtime.Scheme, creds *credentials.Manager, api openapi.Interface, kind string) *Handler {
	h := NewHandler(c, scheme, creds, api, kind)
	h.Mode = ModeMutate
	return h
}

func (h *Handler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if h.isOperator(req) {
		return admission.Allowed("operator request")
	}
	if req.DryRun != nil && *req.DryRun {
		return admission.Allowed("dry run")
	}
	if h.Mode == ModeMutate {
		return h.handleMutating(ctx, req)
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

func (h *Handler) handleMutating(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create {
		return admission.Allowed("mutation ignored")
	}
	switch h.Kind {
	case "SandboxTemplate":
		return h.mutateTemplateCreate(ctx, req)
	case "Sandbox":
		return h.mutateSandboxCreate(ctx, req)
	case "SandboxClaim":
		return h.mutateClaimCreate(ctx, req)
	default:
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("unsupported webhook kind %s", h.Kind))
	}
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
		return admission.Allowed("template create validated")
	case admissionv1.Update:
		var obj sandboxv1.SandboxTemplate
		var old sandboxv1.SandboxTemplate
		if err := h.Decoder.Decode(req, &obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := h.decodeOld(req, &old); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := h.validateReservedAnnotationsUnchanged(&old, &obj); err != nil {
			return admission.Denied(err.Error())
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
		templateID := annotations.Get(obj.Annotations, annotations.TemplateID)
		if templateID == "" {
			return admission.Denied("metadata.annotations[sandbox.kce.ksyun.com/template-id] is empty; wait for OpenAPI sync before updating")
		}
		cred, runtimeCreds, err := h.templateCredentials(ctx, &obj)
		if err != nil {
			return admission.Denied(err.Error())
		}
		updateReq := mapper.TemplateUpdateRequestFromDiff(&obj, &old, runtimeCreds)
		if updateReq.KecConfig != nil && updateReq.KecConfig.Enabled {
			remote, err := h.OpenAPI.GetTemplate(ctx, mapper.OpenAPICredential(cred), templateID)
			if err != nil {
				return admission.Denied(err.Error())
			}
			if err := mapper.CompleteTemplateUpdateRequestFromRemote(&updateReq, *remote); err != nil {
				return admission.Denied(err.Error())
			}
		}
		if mapper.TemplateRequestNeedsStorageCredential(updateReq) && (updateReq.AccessKey == "" || updateReq.SecretAccessKey == "") {
			return admission.Denied("updating KS3/KPFS mount config requires a credentialRef that points to a Secret with accessKey and secretAccessKey")
		}
		if err := h.validateTemplateMountCredentialRefs(&obj, updateReq); err != nil {
			return admission.Denied(err.Error())
		}
		if err := h.OpenAPI.UpdateTemplate(ctx, mapper.OpenAPICredential(cred), updateReq); err != nil {
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
		if err := h.validateSandboxTemplateSource(&obj); err != nil {
			return admission.Denied(err.Error())
		}
		if err := h.validateSandboxName(ctx, &obj, ""); err != nil {
			return admission.Denied(err.Error())
		}
		return admission.Allowed("sandbox create validated")
	case admissionv1.Update:
		var obj sandboxv1.Sandbox
		var old sandboxv1.Sandbox
		if err := h.Decoder.Decode(req, &obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := h.decodeOld(req, &old); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := h.validateReservedAnnotationsUnchanged(&old, &obj); err != nil {
			return admission.Denied(err.Error())
		}
		if err := h.validateSandboxTemplateSource(&obj); err != nil {
			return admission.Denied(err.Error())
		}
		if err := h.validateSandboxName(ctx, &obj, old.Name); err != nil {
			return admission.Denied(err.Error())
		}
		if !sandboxSpecOnlyNameOrTimeoutChanged(old.Spec, obj.Spec) {
			return admission.Denied("only spec.name and spec.timeoutSeconds can be updated on Sandbox")
		}
		if old.Spec.TimeoutSeconds != obj.Spec.TimeoutSeconds {
			if annotations.Get(obj.Annotations, annotations.SandboxID) == "" {
				return admission.Denied("metadata.annotations[sandbox.kce.ksyun.com/sandbox-id] is empty; wait for OpenAPI sync before updating timeout")
			}
			cred, _, err := h.sandboxCredentials(ctx, &obj)
			if err != nil {
				return admission.Denied(err.Error())
			}
			if err := h.OpenAPI.UpdateSandbox(ctx, mapper.OpenAPICredential(cred), mapper.SandboxUpdateRequest(&obj)); err != nil {
				return admission.Denied(err.Error())
			}
			return admission.Allowed("sandbox timeout updated in openapi")
		}
		return admission.Allowed("sandbox update accepted")
	case admissionv1.Delete:
		return admission.Allowed("sandbox deletion handled by finalizer")
	default:
		return admission.Allowed("operation ignored")
	}
}

func (h *Handler) handleClaim(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation == admissionv1.Update {
		var obj sandboxv1.SandboxClaim
		var old sandboxv1.SandboxClaim
		if err := h.Decoder.Decode(req, &obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := h.decodeOld(req, &old); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := h.validateReservedAnnotationsUnchanged(&old, &obj); err != nil {
			return admission.Denied(err.Error())
		}
		return admission.Allowed("claim update accepted")
	}
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
	if _, err := h.resolveClaimTemplateID(ctx, &obj); err != nil {
		return admission.Denied(err.Error())
	}
	for i := 0; i < obj.Spec.Replicas; i++ {
		name := fmt.Sprintf("%s-%d", obj.Name, i)
		if err := h.ensureSandboxNameAvailable(ctx, obj.Namespace, name, ""); err != nil {
			return admission.Denied(err.Error())
		}
	}
	return admission.Allowed("claim create validated")
}

func (h *Handler) mutateTemplateCreate(ctx context.Context, req admission.Request) admission.Response {
	var obj sandboxv1.SandboxTemplate
	if err := h.Decoder.Decode(req, &obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if err := h.validateNoReservedAnnotations(&obj); err != nil {
		return admission.Denied(err.Error())
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
	obj.Annotations = annotations.Set(obj.Annotations, annotations.TemplateID, created.Identifier())
	return h.patch(req, &obj)
}

func (h *Handler) mutateSandboxCreate(ctx context.Context, req admission.Request) admission.Response {
	var obj sandboxv1.Sandbox
	if err := h.Decoder.Decode(req, &obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if err := h.validateNoReservedAnnotations(&obj); err != nil {
		return admission.Denied(err.Error())
	}
	if err := h.validateSandboxName(ctx, &obj, ""); err != nil {
		return admission.Denied(err.Error())
	}
	if err := h.validateSandboxTemplateSource(&obj); err != nil {
		return admission.Denied(err.Error())
	}
	cred, runtimeCreds, err := h.sandboxCredentials(ctx, &obj)
	if err != nil {
		return admission.Denied(err.Error())
	}
	templateID := ""
	if obj.Spec.Template != nil {
		inlineTemplate := mapper.SandboxInlineTemplateObject(&obj)
		if err := h.validateTemplate(inlineTemplate); err != nil {
			return admission.Denied(err.Error())
		}
		templateCred, templateRuntimeCreds, err := h.templateCredentials(ctx, inlineTemplate)
		if err != nil {
			return admission.Denied(err.Error())
		}
		templateReq := mapper.TemplateCreateRequest(inlineTemplate, templateRuntimeCreds)
		if mapper.TemplateCreateRequestNeedsStorageCredential(templateReq) && (templateReq.AccessKey == "" || templateReq.SecretAccessKey == "") {
			return admission.Denied("creating an inline template with KS3/KPFS mount config requires spec.template.spec.storageCredentialRef that points to a Secret with accessKey and secretAccessKey")
		}
		if err := h.validateTemplateCreateMountCredentialRefs(inlineTemplate, templateReq); err != nil {
			return admission.Denied(err.Error())
		}
		created, err := h.OpenAPI.CreateTemplate(ctx, mapper.OpenAPICredential(templateCred), templateReq)
		if err != nil {
			return admission.Denied(err.Error())
		}
		templateID = created.Identifier()
		obj.Annotations = annotations.Set(obj.Annotations, annotations.TemplateID, templateID)
		obj.Annotations = annotations.Set(obj.Annotations, annotations.InlineTemplate, "true")
	} else {
		var err error
		templateID, err = h.resolveTemplateID(ctx, &obj)
		if err != nil {
			return admission.Denied(err.Error())
		}
	}
	startReq := mapper.SandboxStartRequest(&obj, templateID, runtimeCreds)
	if mapper.SandboxRequestNeedsStorageCredential(startReq) && (startReq.AccessKey == "" || startReq.SecretAccessKey == "") {
		return admission.Denied("starting a sandbox with KS3/KPFS mount config requires a credentialRef that points to a Secret with accessKey and secretAccessKey")
	}
	started, err := h.OpenAPI.StartSandbox(ctx, mapper.OpenAPICredential(cred), startReq)
	if err != nil {
		if annotations.Get(obj.Annotations, annotations.InlineTemplate) == "true" && templateID != "" {
			_ = h.OpenAPI.DeleteTemplate(ctx, mapper.OpenAPICredential(cred), templateID)
		}
		return admission.Denied(err.Error())
	}
	obj.Annotations = annotations.Set(obj.Annotations, annotations.TemplateID, started.TemplateIdentifier())
	obj.Annotations = annotations.Set(obj.Annotations, annotations.SandboxID, started.Identifier())
	obj.Annotations = annotations.Set(obj.Annotations, annotations.Endpoint, started.Endpoint)
	obj.Annotations = annotations.Set(obj.Annotations, annotations.Token, started.Token)
	return h.patch(req, &obj)
}

func (h *Handler) mutateClaimCreate(ctx context.Context, req admission.Request) admission.Response {
	var obj sandboxv1.SandboxClaim
	if err := h.Decoder.Decode(req, &obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if err := h.validateNoReservedAnnotations(&obj); err != nil {
		return admission.Denied(err.Error())
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
	var runtimeCreds mapper.RuntimeCredentials
	if obj.Spec.StorageCredentialRef != nil && obj.Spec.StorageCredentialRef.Name != "" {
		runtimeCreds.Storage, err = h.Credentials.GetRuntime(ctx, obj.Namespace, obj.Spec.StorageCredentialRef)
		if err != nil {
			return admission.Denied(err.Error())
		}
	}
	sandboxIDs := make([]string, 0, obj.Spec.Replicas)
	for i := 0; i < obj.Spec.Replicas; i++ {
		name := fmt.Sprintf("%s-%d", obj.Name, i)
		if err := h.ensureSandboxNameAvailable(ctx, obj.Namespace, name, ""); err != nil {
			return admission.Denied(err.Error())
		}
		startReq := mapper.SandboxStartRequest(&sandboxv1.Sandbox{
			Spec: sandboxv1.SandboxSpec{
				TemplateRef:          sandboxv1.TemplateReference{ID: templateID},
				TimeoutSeconds:       obj.Spec.TimeoutSeconds,
				Env:                  obj.Spec.Env,
				Ks3MountConfig:       obj.Spec.Ks3MountConfig,
				KpfsMountConfig:      obj.Spec.KpfsMountConfig,
				StorageCredentialRef: obj.Spec.StorageCredentialRef,
			},
		}, templateID, runtimeCreds)
		started, err := h.OpenAPI.StartSandbox(ctx, mapper.OpenAPICredential(cred), startReq)
		if err != nil {
			return admission.Denied(err.Error())
		}
		sandboxIDs = append(sandboxIDs, started.Identifier())
	}
	obj.Annotations = annotations.Set(obj.Annotations, annotations.TemplateID, templateID)
	obj.Annotations = annotations.Set(obj.Annotations, annotations.SandboxIDs, annotations.EncodeStringSlice(sandboxIDs))
	return h.patch(req, &obj)
}

func (h *Handler) patch(req admission.Request, obj client.Object) admission.Response {
	current, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, current)
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
		if tpl.StorageCredentialRef != nil && tpl.StorageCredentialRef.Name != "" {
			runtimeCreds.Storage, err = h.Credentials.GetRuntime(ctx, obj.Namespace, tpl.StorageCredentialRef)
			if err != nil {
				return nil, mapper.RuntimeCredentials{}, err
			}
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
	if obj.Spec.StorageCredentialRef != nil && obj.Spec.StorageCredentialRef.Name != "" {
		runtimeCreds.Storage, err = h.Credentials.GetRuntime(ctx, obj.Namespace, obj.Spec.StorageCredentialRef)
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
	if obj.Spec.Template != nil {
		tpl := obj.Spec.Template.Spec
		kec := tpl.Kec
		hasKec := kec != nil && (kec.InstanceType != "" || kec.SystemDiskType != "")
		hasSystemDisk := tpl.Resources != nil && !tpl.Resources.Disk.IsZero()
		hasDataDisks := len(tpl.DataDisks) > 0
		if hasKec || hasSystemDisk || hasDataDisks {
			if kec == nil || kec.InstanceType == "" || kec.SystemDiskType == "" || !hasSystemDisk {
				return fmt.Errorf("KEC config requires spec.template.spec.kec.instanceType, spec.template.spec.kec.systemDiskType, and spec.template.spec.resources.disk")
			}
		}
	}
	return nil
}

func (h *Handler) validateNoReservedAnnotations(obj client.Object) error {
	if key, ok := annotations.HasReserved(obj.GetAnnotations()); ok {
		return fmt.Errorf("annotation %s is managed by sandbox-operator and cannot be set by users", key)
	}
	return nil
}

func (h *Handler) validateReservedAnnotationsUnchanged(oldObj, newObj client.Object) error {
	if key, changed := annotations.ReservedChanged(oldObj.GetAnnotations(), newObj.GetAnnotations()); changed {
		return fmt.Errorf("annotation %s is managed by sandbox-operator and cannot be changed by users", key)
	}
	return nil
}

func (h *Handler) validateTemplateMountCredentialRefs(obj *sandboxv1.SandboxTemplate, req openapi.UpdateTemplateRequest) error {
	if obj.Spec.Template == nil {
		return nil
	}
	needStorage := (req.KS3MountConfig != nil && req.KS3MountConfig.EnableKS3) ||
		(req.KPFSMountConfig != nil && req.KPFSMountConfig.EnableKPFS)
	if !needStorage {
		return nil
	}
	tpl := obj.Spec.Template.Spec
	if tpl.StorageCredentialRef == nil || tpl.StorageCredentialRef.Name == "" {
		return fmt.Errorf("KS3/KPFS mount config requires spec.template.spec.storageCredentialRef")
	}
	return nil
}

func (h *Handler) validateTemplateCreateMountCredentialRefs(obj *sandboxv1.SandboxTemplate, req openapi.CreateTemplateRequest) error {
	if obj.Spec.Template == nil {
		return nil
	}
	needStorage := (req.KS3MountConfig != nil && req.KS3MountConfig.EnableKS3) ||
		(req.KPFSMountConfig != nil && req.KPFSMountConfig.EnableKPFS)
	if !needStorage {
		return nil
	}
	tpl := obj.Spec.Template.Spec
	if tpl.StorageCredentialRef == nil || tpl.StorageCredentialRef.Name == "" {
		return fmt.Errorf("KS3/KPFS mount config requires spec.template.spec.storageCredentialRef")
	}
	return nil
}

func (h *Handler) validateSandboxTemplateSource(obj *sandboxv1.Sandbox) error {
	hasTemplateRef := obj.Spec.TemplateRef.ID != "" || obj.Spec.TemplateRef.Name != ""
	hasInlineTemplate := obj.Spec.Template != nil
	if hasTemplateRef && hasInlineTemplate {
		return fmt.Errorf("spec.templateRef and spec.template are mutually exclusive")
	}
	if !hasTemplateRef && !hasInlineTemplate {
		return fmt.Errorf("spec.templateRef or spec.template is required")
	}
	if hasInlineTemplate {
		inlineTemplate := mapper.SandboxInlineTemplateObject(obj)
		if err := h.validateTemplate(inlineTemplate); err != nil {
			return err
		}
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
	templateID := annotations.Get(template.Annotations, annotations.TemplateID)
	if templateID == "" {
		return "", fmt.Errorf("referenced template %s has empty template-id annotation", obj.Spec.TemplateRef.Name)
	}
	return templateID, nil
}

func (h *Handler) resolveClaimTemplateID(ctx context.Context, obj *sandboxv1.SandboxClaim) (string, error) {
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
	templateID := annotations.Get(template.Annotations, annotations.TemplateID)
	if templateID == "" {
		return "", fmt.Errorf("referenced template %s has empty template-id annotation", obj.Spec.TemplateRef.Name)
	}
	return templateID, nil
}

func (h *Handler) validateSandboxName(ctx context.Context, obj *sandboxv1.Sandbox, currentObjectName string) error {
	effectiveName := obj.Spec.Name
	if effectiveName == "" {
		effectiveName = obj.Name
	}
	return h.ensureSandboxNameAvailable(ctx, obj.Namespace, effectiveName, currentObjectName)
}

func sandboxSpecOnlyNameOrTimeoutChanged(oldSpec, newSpec sandboxv1.SandboxSpec) bool {
	oldSpec.Name = newSpec.Name
	oldSpec.TimeoutSeconds = newSpec.TimeoutSeconds
	return reflect.DeepEqual(oldSpec, newSpec)
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

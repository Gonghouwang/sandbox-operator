package controller

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	sandboxv1 "sandbox-operator/api/v1alpha1"
	"sandbox-operator/internal/credentials"
	"sandbox-operator/internal/mapper"
	"sandbox-operator/internal/openapi"
	"sandbox-operator/internal/operation"
	statusutil "sandbox-operator/internal/status"
)

type Poller struct {
	Client        client.Client
	Credentials   *credentials.Manager
	OpenAPI       openapi.Interface
	Operations    *operation.Recorder
	Interval      time.Duration
	PageSize      int
	AdoptExternal bool
}

func (p *Poller) Run(ctx context.Context) error {
	if p.Interval <= 0 {
		p.Interval = 30 * time.Second
	}
	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()

	for {
		if err := p.SyncAll(ctx); err != nil {
			log.FromContext(ctx).Error(err, "sandbox poller sync failed")
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return nil
		}
	}
}

func (p *Poller) Start(ctx context.Context) error {
	return p.Run(ctx)
}

func (p *Poller) SyncAll(ctx context.Context) error {
	var namespaces corev1.NamespaceList
	if err := p.Client.List(ctx, &namespaces); err != nil {
		return err
	}
	for _, ns := range namespaces.Items {
		if err := p.SyncNamespace(ctx, ns.Name); err != nil {
			log.FromContext(ctx).Error(err, "sync namespace failed", "namespace", ns.Name)
		}
	}
	return nil
}

func (p *Poller) SyncNamespace(ctx context.Context, namespace string) error {
	cred, err := p.Credentials.GetOpenAPI(ctx, namespace, nil)
	if err != nil {
		return nil
	}
	openapiCred := mapper.OpenAPICredential(cred)
	if err := p.syncTemplates(ctx, namespace, openapiCred); err != nil {
		return err
	}
	return p.syncSandboxes(ctx, namespace, openapiCred)
}

func (p *Poller) syncTemplates(ctx context.Context, namespace string, cred openapi.Credential) error {
	var local sandboxv1.SandboxTemplateList
	if err := p.Client.List(ctx, &local, client.InNamespace(namespace)); err != nil {
		return err
	}
	knownIDs := map[string]bool{}
	knownNames := map[string]bool{}

	for i := range local.Items {
		obj := &local.Items[i]
		knownNames[obj.Name] = true
		if obj.Status.TemplateID == "" {
			if p.Operations == nil {
				continue
			}
			rec, err := p.Operations.Get(ctx, namespace, "SandboxTemplate", obj.Name)
			if err != nil || rec.TemplateID == "" {
				continue
			}
			obj.Status.TemplateID = rec.TemplateID
		}
		knownIDs[obj.Status.TemplateID] = true
		remote, err := p.OpenAPI.GetTemplate(ctx, cred, obj.Status.TemplateID)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		mapper.ApplyTemplateSpecFromOpenAPI(obj, *remote)
		if err := p.Client.Update(ctx, obj); err != nil {
			return err
		}
		mapper.ApplyTemplateStatusFromOpenAPI(obj, *remote)
		statusutil.SetCondition(&obj.Status.Conditions, sandboxv1.ConditionSynced, "True", "OpenAPISynced", "Template has been synced from Sandbox OpenAPI.", obj.Generation)
		if err := p.Client.Status().Update(ctx, obj); err != nil {
			return err
		}
	}
	if p.AdoptExternal {
		list, err := p.OpenAPI.ListTemplates(ctx, cred, openapi.ListTemplatesRequest{PageNum: 1, PageSize: p.pageSize()})
		if err != nil {
			return err
		}
		for _, remote := range list.Items {
			if remote.TemplateID == "" || knownIDs[remote.TemplateID] {
				continue
			}
			name := uniqueName(sanitizeName(remote.TemplateName, "template-"+shortID(remote.TemplateID)), knownNames)
			obj := &sandboxv1.SandboxTemplate{
				TypeMeta: metav1.TypeMeta{APIVersion: sandboxv1.GroupVersion.String(), Kind: "SandboxTemplate"},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
					Labels: map[string]string{
						"sandbox.kce.ksyun.com/adopted": "true",
					},
				},
			}
			mapper.ApplyTemplateSpecFromOpenAPI(obj, remote)
			if err := p.Client.Create(ctx, obj); err != nil {
				return err
			}
			mapper.ApplyTemplateStatusFromOpenAPI(obj, remote)
			statusutil.SetCondition(&obj.Status.Conditions, sandboxv1.ConditionSynced, "True", "OpenAPIAdopted", "Template has been adopted from Sandbox OpenAPI.", obj.Generation)
			if err := p.Client.Status().Update(ctx, obj); err != nil {
				return err
			}
			knownIDs[remote.TemplateID] = true
			knownNames[name] = true
		}
	}
	return nil
}

func (p *Poller) syncSandboxes(ctx context.Context, namespace string, cred openapi.Credential) error {
	var local sandboxv1.SandboxList
	if err := p.Client.List(ctx, &local, client.InNamespace(namespace)); err != nil {
		return err
	}
	knownIDs := map[string]bool{}
	knownNames := map[string]bool{}

	for i := range local.Items {
		obj := &local.Items[i]
		knownNames[obj.Name] = true
		if obj.Status.SandboxID == "" {
			if p.Operations == nil {
				continue
			}
			rec, err := p.Operations.Get(ctx, namespace, "Sandbox", obj.Name)
			if err != nil || rec.SandboxID == "" {
				continue
			}
			obj.Status.SandboxID = rec.SandboxID
			if rec.Endpoint != "" {
				obj.Status.Endpoint = rec.Endpoint
			}
		}
		knownIDs[obj.Status.SandboxID] = true
		remote, err := p.OpenAPI.GetSandbox(ctx, cred, obj.Status.SandboxID)
		if err != nil {
			return err
		}
		mapper.ApplySandboxSpecFromOpenAPI(obj, *remote)
		if err := p.Client.Update(ctx, obj); err != nil {
			return err
		}
		mapper.ApplySandboxStatusFromOpenAPI(obj, *remote)
		if remote.Status == "RUNNING" {
			token, err := p.OpenAPI.GetSandboxToken(ctx, cred, obj.Status.SandboxID)
			if err != nil {
				log.FromContext(ctx).Error(err, "get sandbox token failed", "namespace", obj.Namespace, "name", obj.Name, "sandboxID", obj.Status.SandboxID)
			} else if token != nil {
				obj.Status.Token = token.Token
			}
		}
		statusutil.SetCondition(&obj.Status.Conditions, sandboxv1.ConditionSynced, "True", "OpenAPISynced", "Sandbox has been synced from Sandbox OpenAPI.", obj.Generation)
		if err := p.Client.Status().Update(ctx, obj); err != nil {
			return err
		}
	}
	if p.AdoptExternal {
		list, err := p.OpenAPI.ListSandboxes(ctx, cred, openapi.ListSandboxesRequest{PageNum: 1, PageSize: p.pageSize()})
		if err != nil {
			return err
		}
		for _, remote := range list.Items {
			if remote.SandboxID == "" || knownIDs[remote.SandboxID] {
				continue
			}
			name := uniqueName("sbx-"+shortID(remote.SandboxID), knownNames)
			obj := &sandboxv1.Sandbox{
				TypeMeta: metav1.TypeMeta{APIVersion: sandboxv1.GroupVersion.String(), Kind: "Sandbox"},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
					Labels: map[string]string{
						"sandbox.kce.ksyun.com/adopted": "true",
					},
				},
				Spec: sandboxv1.SandboxSpec{
					Name:           name,
					TemplateRef:    sandboxv1.TemplateReference{ID: remote.TemplateID},
					TimeoutSeconds: remote.Timeout,
					DeletionPolicy: sandboxv1.DeletionPolicyRetain,
				},
			}
			mapper.ApplySandboxSpecFromOpenAPI(obj, remote)
			if err := p.Client.Create(ctx, obj); err != nil {
				return err
			}
			mapper.ApplySandboxStatusFromOpenAPI(obj, remote)
			statusutil.SetCondition(&obj.Status.Conditions, sandboxv1.ConditionSynced, "True", "OpenAPIAdopted", "Sandbox has been adopted from Sandbox OpenAPI.", obj.Generation)
			if err := p.Client.Status().Update(ctx, obj); err != nil {
				return err
			}
			knownIDs[remote.SandboxID] = true
			knownNames[name] = true
		}
	}
	return nil
}

func (p *Poller) pageSize() int {
	if p.PageSize <= 0 {
		return 100
	}
	return p.PageSize
}

func sanitizeName(value, fallback string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		out = fallback
	}
	if len(out) > 253 {
		out = out[:253]
	}
	return out
}

func uniqueName(base string, used map[string]bool) string {
	if !used[base] {
		return base
	}
	for i := 1; ; i++ {
		suffix := "-" + shortID(time.Now().Format("150405.000000000")) + "-" + string(rune('a'+(i%26)))
		candidate := base
		if len(candidate)+len(suffix) > 253 {
			candidate = candidate[:253-len(suffix)]
		}
		candidate += suffix
		if !used[candidate] {
			return candidate
		}
	}
}

func shortID(id string) string {
	id = sanitizeName(id, "unknown")
	if len(id) <= 12 {
		return id
	}
	return id[len(id)-12:]
}

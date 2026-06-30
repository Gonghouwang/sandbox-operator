package controller

import (
	"context"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
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
	Client                  client.Client
	Credentials             *credentials.Manager
	OpenAPI                 openapi.Interface
	Operations              *operation.Recorder
	Interval                time.Duration
	PageSize                int
	MaxConcurrentNamespaces int
	AdoptExternal           bool
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
	limit := p.MaxConcurrentNamespaces
	if limit <= 0 {
		limit = 5
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for _, ns := range namespaces.Items {
		namespace := ns.Name
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			if err := p.SyncNamespace(ctx, namespace); err != nil {
				log.FromContext(ctx).Error(err, "sync namespace failed", "namespace", namespace)
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
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
		statusBefore := cloneForCompare(obj.Status)
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
			if openapi.IsNotFound(err) {
				continue
			}
			return err
		}
		specBefore := cloneForCompare(obj.Spec)
		mapper.ApplyTemplateSpecFromOpenAPI(obj, *remote)
		if hasChanged(specBefore, obj.Spec) {
			if err := p.Client.Update(ctx, obj); err != nil {
				return err
			}
		}
		mapper.ApplyTemplateStatusFromOpenAPI(obj, *remote)
		statusutil.SetCondition(&obj.Status.Conditions, sandboxv1.ConditionSynced, "True", "OpenAPISynced", "Template has been synced from Sandbox OpenAPI.", obj.Generation)
		if hasChanged(statusBefore, obj.Status) {
			if err := p.Client.Status().Update(ctx, obj); err != nil {
				return err
			}
		}
	}
	if p.AdoptExternal {
		remotes, err := p.listAllTemplates(ctx, cred)
		if err != nil {
			return err
		}
		for _, remote := range remotes {
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
		statusBefore := cloneForCompare(obj.Status)
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
			if rec.Token != "" {
				obj.Status.Token = rec.Token
			}
		}
		knownIDs[obj.Status.SandboxID] = true
		remote, err := p.OpenAPI.GetSandbox(ctx, cred, obj.Status.SandboxID)
		if err != nil {
			if openapi.IsNotFound(err) {
				continue
			}
			return err
		}
		specBefore := cloneForCompare(obj.Spec)
		mapper.ApplySandboxSpecFromOpenAPI(obj, *remote)
		if hasChanged(specBefore, obj.Spec) {
			if err := p.Client.Update(ctx, obj); err != nil {
				return err
			}
		}
		mapper.ApplySandboxStatusFromOpenAPI(obj, *remote)
		statusutil.SetCondition(&obj.Status.Conditions, sandboxv1.ConditionSynced, "True", "OpenAPISynced", "Sandbox has been synced from Sandbox OpenAPI.", obj.Generation)
		if hasChanged(statusBefore, obj.Status) {
			if err := p.Client.Status().Update(ctx, obj); err != nil {
				return err
			}
		}
	}
	if p.AdoptExternal {
		remotes, err := p.listAllSandboxes(ctx, cred)
		if err != nil {
			return err
		}
		for _, remote := range remotes {
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

func (p *Poller) listAllTemplates(ctx context.Context, cred openapi.Credential) ([]openapi.Template, error) {
	pageSize := p.pageSize()
	page := 1
	var out []openapi.Template
	for {
		list, err := p.OpenAPI.ListTemplates(ctx, cred, openapi.ListTemplatesRequest{PageNum: page, PageSize: pageSize})
		if err != nil {
			return nil, err
		}
		out = append(out, list.Items...)
		if len(list.Items) < pageSize || (list.Total > 0 && len(out) >= list.Total) {
			return out, nil
		}
		page++
	}
}

func (p *Poller) listAllSandboxes(ctx context.Context, cred openapi.Credential) ([]openapi.Sandbox, error) {
	pageSize := p.pageSize()
	page := 1
	var out []openapi.Sandbox
	for {
		list, err := p.OpenAPI.ListSandboxes(ctx, cred, openapi.ListSandboxesRequest{PageNum: page, PageSize: pageSize})
		if err != nil {
			return nil, err
		}
		out = append(out, list.Items...)
		if len(list.Items) < pageSize || (list.Total > 0 && len(out) >= list.Total) {
			return out, nil
		}
		page++
	}
}

func (p *Poller) pageSize() int {
	if p.PageSize <= 0 {
		return 100
	}
	if p.PageSize > 100 {
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

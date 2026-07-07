package controller

import (
	"context"
	"strconv"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	sandboxv1 "sandbox-operator/api/v1alpha1"
	"sandbox-operator/internal/annotations"
	"sandbox-operator/internal/credentials"
	"sandbox-operator/internal/mapper"
	"sandbox-operator/internal/openapi"
	statusutil "sandbox-operator/internal/status"
)

const (
	TemplateFinalizer = "sandbox.kce.ksyun.com/sandboxtemplate-finalizer"
	SandboxFinalizer  = "sandbox.kce.ksyun.com/sandbox-finalizer"
	ClaimFinalizer    = "sandbox.kce.ksyun.com/sandboxclaim-finalizer"
	FastRequeue       = 5 * time.Second
)

type SandboxTemplateReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Credentials *credentials.Manager
	OpenAPI     openapi.Interface
}

func (r *SandboxTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var obj sandboxv1.SandboxTemplate
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		if containsString(obj.Finalizers, TemplateFinalizer) {
			if err := r.deleteTemplateFromOpenAPI(ctx, &obj); err != nil {
				logger.Error(err, "delete template from openapi failed")
				if updateErr := r.markTemplateDeleteBlocked(ctx, &obj, err); updateErr != nil {
					return ctrl.Result{}, updateErr
				}
				return ctrl.Result{RequeueAfter: FastRequeue}, nil
			}
			obj.Finalizers = removeString(obj.Finalizers, TemplateFinalizer)
			return ctrl.Result{}, ignoreConflict(r.Update(ctx, &obj))
		}
		return ctrl.Result{}, nil
	}

	if !containsString(obj.Finalizers, TemplateFinalizer) {
		obj.Finalizers = append(obj.Finalizers, TemplateFinalizer)
		if err := r.Update(ctx, &obj); err != nil {
			return ctrl.Result{}, ignoreConflict(err)
		}
		return ctrl.Result{RequeueAfter: FastRequeue}, nil
	}

	if err := r.bindAndSyncTemplate(ctx, &obj); err != nil {
		return ctrl.Result{}, err
	}
	if annotations.Get(obj.Annotations, annotations.TemplateID) == "" {
		return ctrl.Result{RequeueAfter: FastRequeue}, nil
	}
	return ctrl.Result{}, nil
}

func (r *SandboxTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(ctrlcontroller.Options{MaxConcurrentReconciles: 4}).
		For(&sandboxv1.SandboxTemplate{}).
		Complete(r)
}

type SandboxReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Credentials *credentials.Manager
	OpenAPI     openapi.Interface
}

func (r *SandboxReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var obj sandboxv1.Sandbox
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		if containsString(obj.Finalizers, SandboxFinalizer) {
			if err := r.deleteSandboxFromOpenAPI(ctx, &obj); err != nil {
				logger.Error(err, "delete sandbox from openapi failed")
				return ctrl.Result{}, err
			}
			obj.Finalizers = removeString(obj.Finalizers, SandboxFinalizer)
			return ctrl.Result{}, ignoreConflict(r.Update(ctx, &obj))
		}
		return ctrl.Result{}, nil
	}

	if !containsString(obj.Finalizers, SandboxFinalizer) {
		obj.Finalizers = append(obj.Finalizers, SandboxFinalizer)
		if err := r.Update(ctx, &obj); err != nil {
			return ctrl.Result{}, ignoreConflict(err)
		}
		return ctrl.Result{RequeueAfter: FastRequeue}, nil
	}

	if err := r.bindAndSyncSandbox(ctx, &obj); err != nil {
		return ctrl.Result{}, err
	}
	if annotations.Get(obj.Annotations, annotations.SandboxID) == "" {
		return ctrl.Result{RequeueAfter: FastRequeue}, nil
	}
	return ctrl.Result{}, nil
}

func (r *SandboxReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(ctrlcontroller.Options{MaxConcurrentReconciles: 8}).
		For(&sandboxv1.Sandbox{}).
		Complete(r)
}

type SandboxClaimReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *SandboxClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var claim sandboxv1.SandboxClaim
	if err := r.Get(ctx, req.NamespacedName, &claim); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !claim.DeletionTimestamp.IsZero() {
		if containsString(claim.Finalizers, ClaimFinalizer) {
			if err := r.deleteClaimSandboxes(ctx, &claim); err != nil {
				return ctrl.Result{}, err
			}
			claim.Finalizers = removeString(claim.Finalizers, ClaimFinalizer)
			return ctrl.Result{}, ignoreConflict(r.Update(ctx, &claim))
		}
		return ctrl.Result{}, nil
	}

	if !containsString(claim.Finalizers, ClaimFinalizer) {
		claim.Finalizers = append(claim.Finalizers, ClaimFinalizer)
		if err := r.Update(ctx, &claim); err != nil {
			return ctrl.Result{}, ignoreConflict(err)
		}
		return ctrl.Result{RequeueAfter: FastRequeue}, nil
	}

	if isTerminalClaimPhase(claim.Status.Phase) {
		return ctrl.Result{}, nil
	}

	if err := r.ensureClaimSandboxes(ctx, &claim); err != nil {
		return ctrl.Result{}, err
	}
	if annotations.Get(claim.Annotations, annotations.SandboxIDs) != "" {
		delete(claim.Annotations, annotations.SandboxIDs)
		if err := r.Update(ctx, &claim); err != nil {
			return ctrl.Result{}, ignoreConflict(err)
		}
		return ctrl.Result{RequeueAfter: FastRequeue}, nil
	}
	var sandboxes sandboxv1.SandboxList
	if err := r.List(ctx, &sandboxes, client.InNamespace(req.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	statusBefore := cloneForCompare(claim.Status)
	next := claim.Status
	next.ObservedGeneration = claim.Generation
	next.Desired = claim.Spec.Replicas
	next.Sandboxes = next.Sandboxes[:0]
	next.Ready = 0
	next.Failed = 0
	for _, sbx := range sandboxes.Items {
		if sbx.Spec.ClaimRef == nil || sbx.Spec.ClaimRef.Name != claim.Name {
			continue
		}
		if !sbx.DeletionTimestamp.IsZero() {
			continue
		}
		next.Created++
		if sbx.Status.Phase == sandboxv1.PhaseRunning {
			next.Ready++
		}
		if sbx.Status.Phase == sandboxv1.PhaseFailed || sbx.Status.Phase == sandboxv1.PhaseUnhealthy {
			next.Failed++
		}
		next.Sandboxes = append(next.Sandboxes, sandboxv1.ClaimedSandbox{
			Name:  sbx.Name,
			Phase: sbx.Status.Phase,
		})
	}
	next.Created = len(next.Sandboxes)
	if next.Failed > 0 {
		next.Phase = sandboxv1.PhaseFailed
		statusutil.SetCondition(&next.Conditions, sandboxv1.ConditionReady, "False", "SandboxFailed", "One or more sandboxes failed.", claim.Generation)
	} else if next.Desired > 0 && next.Ready == next.Desired {
		next.Phase = sandboxv1.PhaseSuccessful
		statusutil.SetCondition(&next.Conditions, sandboxv1.ConditionReady, "True", "AllSandboxesReady", "All claimed sandboxes are running.", claim.Generation)
		statusutil.SetCondition(&next.Conditions, sandboxv1.ConditionBound, "True", "SandboxesBound", "All claimed sandboxes are bound.", claim.Generation)
	} else {
		next.Phase = sandboxv1.PhasePending
		statusutil.SetCondition(&next.Conditions, sandboxv1.ConditionReady, "False", "WaitingForSandboxes", "Waiting for claimed sandboxes to become ready.", claim.Generation)
	}

	claim.Status = next
	if hasChanged(statusBefore, claim.Status) {
		if err := r.Status().Update(ctx, &claim); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{}, nil
			}
			log.FromContext(ctx).Error(err, "update sandbox claim status failed")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *SandboxTemplateReconciler) bindAndSyncTemplate(ctx context.Context, obj *sandboxv1.SandboxTemplate) error {
	statusBefore := cloneForCompare(obj.Status)
	templateID := annotations.Get(obj.Annotations, annotations.TemplateID)
	if templateID == "" || r.Credentials == nil || r.OpenAPI == nil {
		return nil
	}
	cred, err := r.Credentials.GetOpenAPI(ctx, obj.Namespace, obj.Spec.OpenAPICredentialRef)
	if err != nil {
		return err
	}
	remote, err := r.OpenAPI.GetTemplate(ctx, mapper.OpenAPICredential(cred), templateID)
	if err != nil {
		if openapi.IsNotFound(err) {
			return r.handleMissingTemplate(ctx, obj)
		}
		return err
	}
	specBefore := cloneForCompare(obj.Spec)
	mapper.ApplyTemplateSpecFromOpenAPI(obj, *remote)
	if hasChanged(specBefore, obj.Spec) {
		if err := r.Update(ctx, obj); err != nil {
			return ignoreConflict(err)
		}
	}
	mapper.ApplyTemplateStatusFromOpenAPI(obj, *remote)
	statusutil.SetCondition(&obj.Status.Conditions, sandboxv1.ConditionSynced, "True", "OpenAPISynced", "Template has been synced from Sandbox OpenAPI.", obj.Generation)
	if hasChanged(statusBefore, obj.Status) {
		if err := r.Status().Update(ctx, obj); err != nil {
			return ignoreConflict(err)
		}
	}
	return nil
}

func (r *SandboxReconciler) bindAndSyncSandbox(ctx context.Context, obj *sandboxv1.Sandbox) error {
	statusBefore := cloneForCompare(obj.Status)
	sandboxID := annotations.Get(obj.Annotations, annotations.SandboxID)
	if sandboxID == "" || r.Credentials == nil || r.OpenAPI == nil {
		return nil
	}
	cred, err := r.Credentials.GetOpenAPI(ctx, obj.Namespace, obj.Spec.OpenAPICredentialRef)
	if err != nil {
		return err
	}
	openapiCred := mapper.OpenAPICredential(cred)
	remote, err := r.OpenAPI.GetSandbox(ctx, openapiCred, sandboxID)
	if err != nil {
		if openapi.IsNotFound(err) {
			return r.handleMissingSandbox(ctx, obj)
		}
		return err
	}
	log.FromContext(ctx).V(1).Info("fetched sandbox from openapi",
		append([]any{"sandboxID", sandboxID}, sandboxOpenAPIDebugValues(*remote)...)...)
	specBefore := cloneForCompare(obj.Spec)
	mapper.ApplySandboxSpecFromOpenAPI(obj, *remote)
	if hasChanged(specBefore, obj.Spec) {
		if err := r.Update(ctx, obj); err != nil {
			return ignoreConflict(err)
		}
	}
	mapper.ApplySandboxStatusFromOpenAPI(obj, *remote)
	statusutil.SetCondition(&obj.Status.Conditions, sandboxv1.ConditionSynced, "True", "OpenAPISynced", "Sandbox has been synced from Sandbox OpenAPI.", obj.Generation)
	if hasChanged(statusBefore, obj.Status) {
		if err := r.Status().Update(ctx, obj); err != nil {
			return ignoreConflict(err)
		}
	}
	return nil
}

func (r *SandboxTemplateReconciler) deleteTemplateFromOpenAPI(ctx context.Context, obj *sandboxv1.SandboxTemplate) error {
	templateID := annotations.Get(obj.Annotations, annotations.TemplateID)
	if templateID == "" || r.Credentials == nil || r.OpenAPI == nil {
		return nil
	}
	cred, err := r.Credentials.GetOpenAPI(ctx, obj.Namespace, obj.Spec.OpenAPICredentialRef)
	if err != nil {
		return err
	}
	err = r.OpenAPI.DeleteTemplate(ctx, mapper.OpenAPICredential(cred), templateID)
	if openapi.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SandboxTemplateReconciler) markTemplateDeleteBlocked(ctx context.Context, obj *sandboxv1.SandboxTemplate, cause error) error {
	statusBefore := cloneForCompare(obj.Status)
	obj.Status.CanDelete = false
	statusutil.SetCondition(&obj.Status.Conditions, sandboxv1.ConditionDeleting, "False", "OpenAPIDeleteBlocked", cause.Error(), obj.Generation)
	if hasChanged(statusBefore, obj.Status) {
		if err := r.Status().Update(ctx, obj); err != nil {
			return ignoreConflict(err)
		}
	}
	return nil
}

func (r *SandboxReconciler) deleteSandboxFromOpenAPI(ctx context.Context, obj *sandboxv1.Sandbox) error {
	sandboxID := annotations.Get(obj.Annotations, annotations.SandboxID)
	if r.Credentials == nil || r.OpenAPI == nil {
		return nil
	}
	cred, err := r.Credentials.GetOpenAPI(ctx, obj.Namespace, obj.Spec.OpenAPICredentialRef)
	if err != nil {
		return err
	}
	openapiCred := mapper.OpenAPICredential(cred)
	if sandboxID != "" {
		err = r.OpenAPI.DeleteSandbox(ctx, openapiCred, []string{sandboxID})
		if err != nil && !openapi.IsNotFound(err) {
			return err
		}
	}
	if annotations.Get(obj.Annotations, annotations.InlineTemplate) != "true" {
		return nil
	}
	templateID := annotations.Get(obj.Annotations, annotations.TemplateID)
	if templateID == "" {
		return nil
	}
	err = r.OpenAPI.DeleteTemplate(ctx, openapiCred, templateID)
	if openapi.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SandboxClaimReconciler) deleteClaimSandboxes(ctx context.Context, claim *sandboxv1.SandboxClaim) error {
	var sandboxes sandboxv1.SandboxList
	if err := r.List(ctx, &sandboxes, client.InNamespace(claim.Namespace)); err != nil {
		return err
	}
	for i := range sandboxes.Items {
		sbx := &sandboxes.Items[i]
		if sbx.Spec.ClaimRef == nil || sbx.Spec.ClaimRef.Name != claim.Name {
			continue
		}
		if err := r.Delete(ctx, sbx); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (r *SandboxClaimReconciler) ensureClaimSandboxes(ctx context.Context, claim *sandboxv1.SandboxClaim) error {
	sandboxIDs := annotations.DecodeStringSlice(claim.Annotations[annotations.SandboxIDs])
	for i, sandboxID := range sandboxIDs {
		name := claim.Name + "-" + strconv.Itoa(i)
		templateID := annotations.Get(claim.Annotations, annotations.TemplateID)
		var existing sandboxv1.Sandbox
		err := r.Get(ctx, client.ObjectKey{Namespace: claim.Namespace, Name: name}, &existing)
		if err == nil {
			if !existing.DeletionTimestamp.IsZero() {
				continue
			}
			changed := false
			if annotations.Get(existing.Annotations, annotations.SandboxID) == "" {
				if existing.Annotations == nil {
					existing.Annotations = map[string]string{}
				}
				existing.Annotations[annotations.SandboxID] = sandboxID
				changed = true
			}
			if templateID != "" && annotations.Get(existing.Annotations, annotations.TemplateID) == "" {
				if existing.Annotations == nil {
					existing.Annotations = map[string]string{}
				}
				existing.Annotations[annotations.TemplateID] = templateID
				changed = true
			}
			if changed {
				_ = ignoreConflict(r.Update(ctx, &existing))
			}
			continue
		}
		if !apierrors.IsNotFound(err) {
			return err
		}
		obj := &sandboxv1.Sandbox{
			TypeMeta: metav1.TypeMeta{
				APIVersion: sandboxv1.GroupVersion.String(),
				Kind:       "Sandbox",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: claim.Namespace,
				Name:      name,
				Labels: map[string]string{
					"sandbox.kce.ksyun.com/claim": claim.Name,
				},
				Annotations: map[string]string{
					annotations.SandboxID: sandboxID,
				},
			},
			Spec: sandboxv1.SandboxSpec{
				OpenAPICredentialRef: claim.Spec.OpenAPICredentialRef,
				ClaimRef:             &sandboxv1.ClaimReference{Name: claim.Name},
				TemplateRef:          claim.Spec.TemplateRef,
				TimeoutSeconds:       claim.Spec.TimeoutSeconds,
				Env:                  append([]sandboxv1.EnvVar(nil), claim.Spec.Env...),
				StorageCredentialRef: claim.Spec.StorageCredentialRef,
				Ks3MountConfig:       claim.Spec.Ks3MountConfig,
				KpfsMountConfig:      claim.Spec.KpfsMountConfig,
			},
		}
		if templateID != "" {
			obj.Annotations[annotations.TemplateID] = templateID
		}
		if err := r.Create(ctx, obj); err != nil {
			return err
		}
	}
	return nil
}

func isTerminalClaimPhase(phase sandboxv1.Phase) bool {
	return phase == sandboxv1.PhaseSuccessful || phase == sandboxv1.PhaseFailed
}

func (r *SandboxTemplateReconciler) handleMissingTemplate(ctx context.Context, obj *sandboxv1.SandboxTemplate) error {
	return client.IgnoreNotFound(r.Delete(ctx, obj))
}

func (r *SandboxReconciler) handleMissingSandbox(ctx context.Context, obj *sandboxv1.Sandbox) error {
	return client.IgnoreNotFound(r.Delete(ctx, obj))
}

func (r *SandboxClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(ctrlcontroller.Options{MaxConcurrentReconciles: 4}).
		For(&sandboxv1.SandboxClaim{}).
		Watches(&sandboxv1.Sandbox{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			sbx, ok := obj.(*sandboxv1.Sandbox)
			if !ok || sbx.Spec.ClaimRef == nil || sbx.Spec.ClaimRef.Name == "" {
				return nil
			}
			return []reconcile.Request{{
				NamespacedName: client.ObjectKey{Namespace: sbx.Namespace, Name: sbx.Spec.ClaimRef.Name},
			}}
		})).
		Complete(r)
}

type OpenAPIClientProvider interface {
	Client() openapi.Interface
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func removeString(items []string, value string) []string {
	out := items[:0]
	for _, item := range items {
		if item != value {
			out = append(out, item)
		}
	}
	return out
}

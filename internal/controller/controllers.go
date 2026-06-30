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
	"sandbox-operator/internal/credentials"
	"sandbox-operator/internal/mapper"
	"sandbox-operator/internal/openapi"
	"sandbox-operator/internal/operation"
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
	Operations  *operation.Recorder
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
				return ctrl.Result{}, err
			}
			obj.Finalizers = removeString(obj.Finalizers, TemplateFinalizer)
			return ctrl.Result{}, r.Update(ctx, &obj)
		}
		return ctrl.Result{}, nil
	}

	if !containsString(obj.Finalizers, TemplateFinalizer) {
		obj.Finalizers = append(obj.Finalizers, TemplateFinalizer)
		if err := r.Update(ctx, &obj); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: FastRequeue}, nil
	}

	if err := r.bindAndSyncTemplate(ctx, &obj); err != nil {
		return ctrl.Result{}, err
	}
	if obj.Status.TemplateID == "" {
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
	Operations  *operation.Recorder
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
			return ctrl.Result{}, r.Update(ctx, &obj)
		}
		return ctrl.Result{}, nil
	}

	if !containsString(obj.Finalizers, SandboxFinalizer) {
		obj.Finalizers = append(obj.Finalizers, SandboxFinalizer)
		if err := r.Update(ctx, &obj); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: FastRequeue}, nil
	}

	if err := r.bindAndSyncSandbox(ctx, &obj); err != nil {
		return ctrl.Result{}, err
	}
	if obj.Status.SandboxID == "" {
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
	Scheme     *runtime.Scheme
	Operations *operation.Recorder
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
			if r.Operations != nil {
				_ = r.Operations.Delete(ctx, claim.Namespace, "SandboxClaim", claim.Name)
			}
			claim.Finalizers = removeString(claim.Finalizers, ClaimFinalizer)
			return ctrl.Result{}, r.Update(ctx, &claim)
		}
		return ctrl.Result{}, nil
	}

	if !containsString(claim.Finalizers, ClaimFinalizer) {
		claim.Finalizers = append(claim.Finalizers, ClaimFinalizer)
		if err := r.Update(ctx, &claim); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: FastRequeue}, nil
	}

	if err := r.ensureClaimSandboxes(ctx, &claim); err != nil {
		return ctrl.Result{}, err
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
		next.Created++
		if sbx.Status.Phase == sandboxv1.PhaseRunning {
			next.Ready++
		}
		if sbx.Status.Phase == sandboxv1.PhaseFailed || sbx.Status.Phase == sandboxv1.PhaseUnhealthy {
			next.Failed++
		}
		next.Sandboxes = append(next.Sandboxes, sandboxv1.ClaimedSandbox{
			Name:      sbx.Name,
			SandboxID: sbx.Status.SandboxID,
			Phase:     sbx.Status.Phase,
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
			log.FromContext(ctx).Error(err, "update sandbox claim status failed")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *SandboxTemplateReconciler) bindAndSyncTemplate(ctx context.Context, obj *sandboxv1.SandboxTemplate) error {
	statusBefore := cloneForCompare(obj.Status)
	if r.Operations != nil && obj.Status.TemplateID == "" {
		rec, err := r.Operations.Get(ctx, obj.Namespace, "SandboxTemplate", obj.Name)
		if err == nil && rec.TemplateID != "" {
			obj.Status.TemplateID = rec.TemplateID
		}
	}
	if obj.Status.TemplateID == "" || r.Credentials == nil || r.OpenAPI == nil {
		return nil
	}
	cred, err := r.Credentials.GetOpenAPI(ctx, obj.Namespace, obj.Spec.OpenAPICredentialRef)
	if err != nil {
		return err
	}
	remote, err := r.OpenAPI.GetTemplate(ctx, mapper.OpenAPICredential(cred), obj.Status.TemplateID)
	if err != nil {
		return err
	}
	specBefore := cloneForCompare(obj.Spec)
	mapper.ApplyTemplateSpecFromOpenAPI(obj, *remote)
	if hasChanged(specBefore, obj.Spec) {
		if err := r.Update(ctx, obj); err != nil {
			return err
		}
	}
	mapper.ApplyTemplateStatusFromOpenAPI(obj, *remote)
	statusutil.SetCondition(&obj.Status.Conditions, sandboxv1.ConditionSynced, "True", "OpenAPISynced", "Template has been synced from Sandbox OpenAPI.", obj.Generation)
	if hasChanged(statusBefore, obj.Status) {
		if err := r.Status().Update(ctx, obj); err != nil {
			return err
		}
	}
	if r.Operations != nil {
		_ = r.Operations.Delete(ctx, obj.Namespace, "SandboxTemplate", obj.Name)
	}
	return nil
}

func (r *SandboxReconciler) bindAndSyncSandbox(ctx context.Context, obj *sandboxv1.Sandbox) error {
	statusBefore := cloneForCompare(obj.Status)
	if r.Operations != nil && obj.Status.SandboxID == "" {
		rec, err := r.Operations.Get(ctx, obj.Namespace, "Sandbox", obj.Name)
		if err == nil && rec.SandboxID != "" {
			obj.Status.SandboxID = rec.SandboxID
			obj.Status.Endpoint = rec.Endpoint
			obj.Status.Token = rec.Token
		}
	}
	if obj.Status.SandboxID == "" || r.Credentials == nil || r.OpenAPI == nil {
		return nil
	}
	cred, err := r.Credentials.GetOpenAPI(ctx, obj.Namespace, obj.Spec.OpenAPICredentialRef)
	if err != nil {
		return err
	}
	openapiCred := mapper.OpenAPICredential(cred)
	remote, err := r.OpenAPI.GetSandbox(ctx, openapiCred, obj.Status.SandboxID)
	if err != nil {
		return err
	}
	specBefore := cloneForCompare(obj.Spec)
	mapper.ApplySandboxSpecFromOpenAPI(obj, *remote)
	if hasChanged(specBefore, obj.Spec) {
		if err := r.Update(ctx, obj); err != nil {
			return err
		}
	}
	mapper.ApplySandboxStatusFromOpenAPI(obj, *remote)
	statusutil.SetCondition(&obj.Status.Conditions, sandboxv1.ConditionSynced, "True", "OpenAPISynced", "Sandbox has been synced from Sandbox OpenAPI.", obj.Generation)
	if hasChanged(statusBefore, obj.Status) {
		if err := r.Status().Update(ctx, obj); err != nil {
			return err
		}
	}
	if r.Operations != nil {
		_ = r.Operations.Delete(ctx, obj.Namespace, "Sandbox", obj.Name)
	}
	return nil
}

func (r *SandboxTemplateReconciler) deleteTemplateFromOpenAPI(ctx context.Context, obj *sandboxv1.SandboxTemplate) error {
	if obj.Spec.DeletionPolicy == sandboxv1.DeletionPolicyRetain || obj.Status.TemplateID == "" || r.Credentials == nil || r.OpenAPI == nil {
		return nil
	}
	cred, err := r.Credentials.GetOpenAPI(ctx, obj.Namespace, obj.Spec.OpenAPICredentialRef)
	if err != nil {
		return err
	}
	err = r.OpenAPI.DeleteTemplate(ctx, mapper.OpenAPICredential(cred), obj.Status.TemplateID)
	if openapi.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SandboxReconciler) deleteSandboxFromOpenAPI(ctx context.Context, obj *sandboxv1.Sandbox) error {
	if obj.Spec.DeletionPolicy == sandboxv1.DeletionPolicyRetain || obj.Status.SandboxID == "" || r.Credentials == nil || r.OpenAPI == nil {
		return nil
	}
	cred, err := r.Credentials.GetOpenAPI(ctx, obj.Namespace, obj.Spec.OpenAPICredentialRef)
	if err != nil {
		return err
	}
	err = r.OpenAPI.DeleteSandbox(ctx, mapper.OpenAPICredential(cred), []string{obj.Status.SandboxID})
	if openapi.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SandboxClaimReconciler) deleteClaimSandboxes(ctx context.Context, claim *sandboxv1.SandboxClaim) error {
	if claim.Spec.DeletionPolicy == sandboxv1.DeletionPolicyRetain {
		return nil
	}
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
	if r.Operations == nil {
		return nil
	}
	rec, err := r.Operations.Get(ctx, claim.Namespace, "SandboxClaim", claim.Name)
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	for i, sandboxID := range rec.SandboxIDs {
		name := claim.Name + "-" + strconv.Itoa(i)
		var existing sandboxv1.Sandbox
		err := r.Get(ctx, client.ObjectKey{Namespace: claim.Namespace, Name: name}, &existing)
		if err == nil {
			if existing.Status.SandboxID == "" {
				existing.Status.SandboxID = sandboxID
				_ = r.Status().Update(ctx, &existing)
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
			},
			Spec: sandboxv1.SandboxSpec{
				Name:                 name,
				OpenAPICredentialRef: claim.Spec.OpenAPICredentialRef,
				ClaimRef:             &sandboxv1.ClaimReference{Name: claim.Name},
				TemplateRef:          claim.Spec.TemplateRef,
				TimeoutSeconds:       claim.Spec.TimeoutSeconds,
				Env:                  append([]sandboxv1.EnvVar(nil), claim.Spec.Env...),
				Ks3MountConfig:       claim.Spec.Ks3MountConfig,
				KpfsMountConfig:      claim.Spec.KpfsMountConfig,
				DeletionPolicy:       claim.Spec.DeletionPolicy,
			},
		}
		if err := r.Create(ctx, obj); err != nil {
			return err
		}
		obj.Status.SandboxID = sandboxID
		obj.Status.ObservedGeneration = obj.Generation
		if err := r.Status().Update(ctx, obj); err != nil {
			return err
		}
	}
	return nil
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

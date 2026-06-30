package controller

import (
	"context"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	sandboxv1 "sandbox-operator/api/v1alpha1"
	"sandbox-operator/internal/openapi"
	"sandbox-operator/internal/operation"
	statusutil "sandbox-operator/internal/status"
)

type SandboxTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *SandboxTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *SandboxTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&sandboxv1.SandboxTemplate{}).
		Complete(r)
}

type SandboxReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *SandboxReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *SandboxReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
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
	if err := r.ensureClaimSandboxes(ctx, &claim); err != nil {
		return ctrl.Result{}, err
	}

	var sandboxes sandboxv1.SandboxList
	if err := r.List(ctx, &sandboxes, client.InNamespace(req.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

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
	if err := r.Status().Update(ctx, &claim); err != nil {
		log.FromContext(ctx).Error(err, "update sandbox claim status failed")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
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
		For(&sandboxv1.SandboxClaim{}).
		Complete(r)
}

type OpenAPIClientProvider interface {
	Client() openapi.Interface
}

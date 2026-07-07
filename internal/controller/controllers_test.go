package controller

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	sandboxv1 "sandbox-operator/api/v1alpha1"
	"sandbox-operator/internal/annotations"
)

func TestSandboxClaimConsumesSandboxIDsAndDoesNotRecreateExpiredChild(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := sandboxv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	claim := &sandboxv1.SandboxClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "full-claim",
			Namespace: "sandbox-demo",
			Finalizers: []string{
				ClaimFinalizer,
			},
			Annotations: map[string]string{
				annotations.TemplateID: "template-1",
				annotations.SandboxIDs: annotations.EncodeStringSlice([]string{"sandbox-1"}),
			},
		},
		Spec: sandboxv1.SandboxClaimSpec{
			Replicas:    1,
			TemplateRef: sandboxv1.TemplateReference{Name: "full-template"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(claim).
		WithStatusSubresource(&sandboxv1.SandboxClaim{}).
		Build()
	reconciler := &SandboxClaimReconciler{Client: c, Scheme: scheme}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: claim.Namespace, Name: claim.Name}}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	var updatedClaim sandboxv1.SandboxClaim
	if err := c.Get(ctx, request.NamespacedName, &updatedClaim); err != nil {
		t.Fatalf("get claim after first reconcile: %v", err)
	}
	if annotations.Get(updatedClaim.Annotations, annotations.SandboxIDs) != "" {
		t.Fatalf("sandbox-ids annotation should be consumed, got %q", updatedClaim.Annotations[annotations.SandboxIDs])
	}

	childKey := types.NamespacedName{Namespace: claim.Namespace, Name: "full-claim-0"}
	var child sandboxv1.Sandbox
	if err := c.Get(ctx, childKey, &child); err != nil {
		t.Fatalf("expected child sandbox to be created: %v", err)
	}
	if got := annotations.Get(child.Annotations, annotations.SandboxID); got != "sandbox-1" {
		t.Fatalf("unexpected child sandbox-id annotation: %q", got)
	}

	updatedClaim.Status.Desired = 1
	updatedClaim.Status.Created = 1
	updatedClaim.Status.Phase = sandboxv1.PhaseSuccessful
	if err := c.Status().Update(ctx, &updatedClaim); err != nil {
		t.Fatalf("seed materialized claim status: %v", err)
	}
	if err := c.Delete(ctx, &child); err != nil {
		t.Fatalf("delete child sandbox: %v", err)
	}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}
	if err := c.Get(ctx, childKey, &child); !apierrors.IsNotFound(err) {
		t.Fatalf("expired child sandbox should not be recreated, get err=%v", err)
	}

	if err := c.Get(ctx, request.NamespacedName, &updatedClaim); err != nil {
		t.Fatalf("get claim after second reconcile: %v", err)
	}
	if updatedClaim.Status.Phase != sandboxv1.PhaseSuccessful {
		t.Fatalf("terminal claim phase should remain unchanged, got %q", updatedClaim.Status.Phase)
	}
}

func TestTerminalSandboxClaimDoesNotMaterializeOrMutate(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := sandboxv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	claim := &sandboxv1.SandboxClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "terminal-claim",
			Namespace: "sandbox-demo",
			Finalizers: []string{
				ClaimFinalizer,
			},
			Annotations: map[string]string{
				annotations.TemplateID: "template-1",
				annotations.SandboxIDs: annotations.EncodeStringSlice([]string{"sandbox-1"}),
			},
		},
		Spec: sandboxv1.SandboxClaimSpec{
			Replicas:    1,
			TemplateRef: sandboxv1.TemplateReference{Name: "full-template"},
		},
		Status: sandboxv1.SandboxClaimStatus{
			Phase:   sandboxv1.PhaseSuccessful,
			Desired: 1,
			Created: 1,
			Ready:   1,
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(claim).
		WithStatusSubresource(&sandboxv1.SandboxClaim{}).
		Build()
	reconciler := &SandboxClaimReconciler{Client: c, Scheme: scheme}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: claim.Namespace, Name: claim.Name}}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("reconcile terminal claim failed: %v", err)
	}

	var child sandboxv1.Sandbox
	childKey := types.NamespacedName{Namespace: claim.Namespace, Name: "terminal-claim-0"}
	if err := c.Get(ctx, childKey, &child); !apierrors.IsNotFound(err) {
		t.Fatalf("terminal claim should not materialize child, get err=%v", err)
	}

	var updatedClaim sandboxv1.SandboxClaim
	if err := c.Get(ctx, request.NamespacedName, &updatedClaim); err != nil {
		t.Fatalf("get terminal claim after reconcile: %v", err)
	}
	if annotations.Get(updatedClaim.Annotations, annotations.SandboxIDs) == "" {
		t.Fatalf("terminal claim should not be mutated")
	}
	if updatedClaim.Status.Phase != sandboxv1.PhaseSuccessful || updatedClaim.Status.Ready != 1 {
		t.Fatalf("terminal claim status should remain unchanged: %#v", updatedClaim.Status)
	}
}

func TestDeletingSandboxClaimDoesNotDeleteClaimedSandboxes(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := sandboxv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	now := metav1.Now()
	claim := &sandboxv1.SandboxClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "one-shot",
			Namespace:         "sandbox-demo",
			Finalizers:        []string{ClaimFinalizer},
			DeletionTimestamp: &now,
		},
		Spec: sandboxv1.SandboxClaimSpec{
			Replicas:    1,
			TemplateRef: sandboxv1.TemplateReference{Name: "full-template"},
		},
	}
	child := &sandboxv1.Sandbox{
		ObjectMeta: metav1.ObjectMeta{Name: "one-shot-0", Namespace: "sandbox-demo"},
		Spec: sandboxv1.SandboxSpec{
			ClaimRef: &sandboxv1.ClaimReference{Name: "one-shot"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(claim, child).
		WithStatusSubresource(&sandboxv1.SandboxClaim{}, &sandboxv1.Sandbox{}).
		Build()
	reconciler := &SandboxClaimReconciler{Client: c, Scheme: scheme}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: claim.Namespace, Name: claim.Name}}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("reconcile deleting claim failed: %v", err)
	}

	var gotChild sandboxv1.Sandbox
	if err := c.Get(ctx, types.NamespacedName{Namespace: child.Namespace, Name: child.Name}, &gotChild); err != nil {
		t.Fatalf("deleting claim should not delete claimed sandbox: %v", err)
	}
	var gotClaim sandboxv1.SandboxClaim
	if err := c.Get(ctx, request.NamespacedName, &gotClaim); !apierrors.IsNotFound(err) {
		t.Fatalf("claim should be removed after finalizer cleanup, get err=%v", err)
	}
}

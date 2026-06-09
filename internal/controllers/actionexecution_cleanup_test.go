package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

func TestActionExecutionCleanupTask_Cleanup(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	now := time.Now()

	// Expired succeeded execution
	succeededExpired := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "succeeded-expired", Namespace: ns},
		Status: operatorv1.ActionExecutionStatus{
			State:   operatorv1.ActionExecutionSucceeded,
			EndedAt: &metav1.Time{Time: now.Add(-8 * 24 * time.Hour)},
		},
	}

	// Non-expired succeeded execution
	succeededFresh := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "succeeded-fresh", Namespace: ns},
		Status: operatorv1.ActionExecutionStatus{
			State:   operatorv1.ActionExecutionSucceeded,
			EndedAt: &metav1.Time{Time: now.Add(-1 * 24 * time.Hour)},
		},
	}

	// Expired failed execution
	failedExpired := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "failed-expired", Namespace: ns},
		Status: operatorv1.ActionExecutionStatus{
			State:   operatorv1.ActionExecutionFailed,
			EndedAt: &metav1.Time{Time: now.Add(-31 * 24 * time.Hour)},
		},
	}

	// Non-expired failed execution
	failedFresh := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "failed-fresh", Namespace: ns},
		Status: operatorv1.ActionExecutionStatus{
			State:   operatorv1.ActionExecutionFailed,
			EndedAt: &metav1.Time{Time: now.Add(-1 * 24 * time.Hour)},
		},
	}

	// Expired timed out execution
	timedOutExpired := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "timedout-expired", Namespace: ns},
		Status: operatorv1.ActionExecutionStatus{
			State:   operatorv1.ActionExecutionTimedOut,
			EndedAt: &metav1.Time{Time: now.Add(-31 * 24 * time.Hour)},
		},
	}

	// Expired rejected execution
	rejectedExpired := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "rejected-expired", Namespace: ns},
		Status: operatorv1.ActionExecutionStatus{
			State:   operatorv1.ActionExecutionRejected,
			EndedAt: &metav1.Time{Time: now.Add(-31 * 24 * time.Hour)},
		},
	}

	// Pending execution (should not be deleted)
	pending := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: ns},
		Status: operatorv1.ActionExecutionStatus{
			State: operatorv1.ActionExecutionPending,
		},
	}

	// Running execution with old endedAt (should not be deleted because state is Running)
	running := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "running", Namespace: ns},
		Status: operatorv1.ActionExecutionStatus{
			State:   operatorv1.ActionExecutionRunning,
			EndedAt: &metav1.Time{Time: now.Add(-31 * 24 * time.Hour)},
		},
	}

	// Completed but no EndedAt (should not be deleted)
	noEndedAt := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "no-ended-at", Namespace: ns},
		Status: operatorv1.ActionExecutionStatus{
			State: operatorv1.ActionExecutionSucceeded,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		succeededExpired, succeededFresh,
		failedExpired, failedFresh,
		timedOutExpired, rejectedExpired,
		pending, running, noEndedAt,
	).Build()

	task := &ActionExecutionCleanupTask{Client: cl}
	err := task.cleanup(ctx)
	require.NoError(t, err)

	list := &operatorv1.ActionExecutionList{}
	require.NoError(t, cl.List(ctx, list))

	names := make(map[string]struct{})
	for _, item := range list.Items {
		names[item.Name] = struct{}{}
	}

	// Should be deleted
	assert.NotContains(t, names, "succeeded-expired")
	assert.NotContains(t, names, "failed-expired")
	assert.NotContains(t, names, "timedout-expired")
	assert.NotContains(t, names, "rejected-expired")

	// Should be kept
	assert.Contains(t, names, "succeeded-fresh")
	assert.Contains(t, names, "failed-fresh")
	assert.Contains(t, names, "pending")
	assert.Contains(t, names, "running")
	assert.Contains(t, names, "no-ended-at")
}

func TestActionExecutionCleanupTask_Cleanup_ListError(t *testing.T) {
	badScheme := runtime.NewScheme()
	cl := fake.NewClientBuilder().WithScheme(badScheme).Build()
	task := &ActionExecutionCleanupTask{Client: cl}

	err := task.cleanup(context.Background())
	require.Error(t, err)
}

func TestActionExecutionCleanupTask_Start(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx, cancel := context.WithCancel(context.Background())

	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	task := &ActionExecutionCleanupTask{Client: cl}

	// Start the task and cancel immediately to test the loop exits cleanly.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := task.Start(ctx)
	require.NoError(t, err)
}

func TestActionExecutionCleanupTask_SetupWithManager(t *testing.T) {
	task := &ActionExecutionCleanupTask{}
	assert.NotNil(t, task)
}

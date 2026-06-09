// Package controllers contains the Kubernetes controllers for minato CRDs.
package controllers

import (
	"context"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

const (
	actionExecutionTTLCleanupInterval = 1 * time.Hour
	actionExecutionSuccessTTL         = 7 * 24 * time.Hour
	actionExecutionFailedTTL          = 30 * 24 * time.Hour
)

type ActionExecutionCleanupTask struct {
	client.Client
}

func (t *ActionExecutionCleanupTask) Start(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.Info("starting ActionExecution cleanup task")

	ticker := time.NewTicker(actionExecutionTTLCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := t.cleanup(ctx); err != nil {
				logger.Error(err, "ActionExecution cleanup failed")
			}
		}
	}
}

func (t *ActionExecutionCleanupTask) cleanup(ctx context.Context) error {
	logger := log.FromContext(ctx)

	var list operatorv1.ActionExecutionList
	if err := t.List(ctx, &list); err != nil {
		return err
	}

	now := time.Now()
	for _, exec := range list.Items {
		if exec.Status.State != operatorv1.ActionExecutionSucceeded &&
			exec.Status.State != operatorv1.ActionExecutionFailed &&
			exec.Status.State != operatorv1.ActionExecutionTimedOut &&
			exec.Status.State != operatorv1.ActionExecutionRejected {
			continue
		}

		if exec.Status.EndedAt == nil {
			continue
		}

		var ttl time.Duration
		switch exec.Status.State {
		case operatorv1.ActionExecutionSucceeded:
			ttl = actionExecutionSuccessTTL
		default:
			ttl = actionExecutionFailedTTL
		}

		if now.Sub(exec.Status.EndedAt.Time) > ttl {
			logger.Info("deleting expired ActionExecution",
				"name", exec.Name, "namespace", exec.Namespace, "state", exec.Status.State)
			if err := t.Delete(ctx, &exec); err != nil {
				logger.Error(err, "failed to delete ActionExecution", "name", exec.Name)
			}
		}
	}

	return nil
}

func (t *ActionExecutionCleanupTask) SetupWithManager(mgr ctrl.Manager) error {
	return mgr.Add(t)
}

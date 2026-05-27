package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunStepsStopsOnError(t *testing.T) {
	steps := []Step{
		func(ctx context.Context) error { return nil },
		func(ctx context.Context) error { return errors.New("fail") },
		func(ctx context.Context) error { return nil },
	}
	if err := RunSteps(context.Background(), steps); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunStepsAllSuccess(t *testing.T) {
	called := 0
	steps := []Step{
		func(ctx context.Context) error { called++; return nil },
		func(ctx context.Context) error { called++; return nil },
		func(ctx context.Context) error { called++; return nil },
	}
	if err := RunSteps(context.Background(), steps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called != 3 {
		t.Fatalf("expected 3 steps called, got %d", called)
	}
}

func TestRunStepsEmpty(t *testing.T) {
	if err := RunSteps(context.Background(), []Step{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunStepsNil(t *testing.T) {
	if err := RunSteps(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunStepsFirstError(t *testing.T) {
	called := 0
	steps := []Step{
		func(ctx context.Context) error { called++; return errors.New("first fail") },
		func(ctx context.Context) error { called++; return nil },
	}
	if err := RunSteps(context.Background(), steps); err == nil {
		t.Fatalf("expected error")
	}
	if called != 1 {
		t.Fatalf("expected 1 step called, got %d", called)
	}
}

func TestRunStepsTimeout(t *testing.T) {
	steps := []Step{
		func(ctx context.Context) error {
			select {
			case <-time.After(100 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := RunSteps(ctx, steps); err == nil {
		t.Fatalf("expected timeout error")
	}
}

func TestRunStepsContextCancellation(t *testing.T) {
	steps := []Step{
		func(ctx context.Context) error {
			return ctx.Err()
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := RunSteps(ctx, steps); err == nil {
		t.Fatalf("expected cancelled error")
	}
}

func TestRunStepsErrorPropagation(t *testing.T) {
	expectedErr := errors.New("propagated")
	steps := []Step{
		func(ctx context.Context) error { return expectedErr },
	}
	err := RunSteps(context.Background(), steps)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
}

func TestRunStepsSingleStep(t *testing.T) {
	called := false
	steps := []Step{
		func(ctx context.Context) error { called = true; return nil },
	}
	if err := RunSteps(context.Background(), steps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("expected step to be called")
	}
}

func TestRunStepsMultipleErrors(t *testing.T) {
	called := 0
	steps := []Step{
		func(ctx context.Context) error { called++; return errors.New("fail1") },
		func(ctx context.Context) error { called++; return errors.New("fail2") },
	}
	err := RunSteps(context.Background(), steps)
	if err == nil {
		t.Fatalf("expected error")
	}
	if called != 1 {
		t.Fatalf("expected 1 step called before error, got %d", called)
	}
}

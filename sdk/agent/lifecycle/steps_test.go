package lifecycle

import (
	"context"
	"errors"
	"testing"
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

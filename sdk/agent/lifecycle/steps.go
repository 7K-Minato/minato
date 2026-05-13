package lifecycle

import (
	"context"
)

type Step func(ctx context.Context) error

func RunSteps(ctx context.Context, steps []Step) error {
	for _, step := range steps {
		if err := step(ctx); err != nil {
			return err
		}
	}
	return nil
}

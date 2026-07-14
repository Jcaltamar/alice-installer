package update

import (
	"context"
	"io"
)

// Action adapts the established update flow for the interactive menu.
type Action struct {
	Config       Config
	Dependencies Dependencies
}

func (a Action) Run(ctx context.Context) error {
	return Run(ctx, a.Config, a.Dependencies, io.Discard)
}

package command

import (
	"context"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
)

// NormalizeFunc converts a domain command type plus params into the registry key
// and normalized parameter map used for execution.
type NormalizeFunc func(domain.CommandType, map[string]any) (string, map[string]any)

type sequentialDispatcher struct {
	registry  *Registry
	normalize NormalizeFunc
}

// NewSequentialDispatcher creates a dispatcher that executes command events in
// the order they are received.
func NewSequentialDispatcher(registry *Registry, normalize NormalizeFunc) Dispatcher {
	return &sequentialDispatcher{registry: registry, normalize: normalize}
}

func (d *sequentialDispatcher) Publish(ctx context.Context, cmdCtx *domain.CommandContext, events ...CommandEvent) (int, error) {
	if d == nil || d.registry == nil || d.normalize == nil {
		return 0, nil
	}

	executed := 0
	for _, event := range events {
		if event.Type == domain.CommandUnknown {
			continue
		}

		normalizedParams := cloneParams(event.Params)
		key, params := d.normalize(event.Type, normalizedParams)
		if err := d.registry.Execute(ctx, cmdCtx, key, params); err != nil {
			return executed, err
		}
		executed++
	}
	return executed, nil
}

func cloneParams(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	clone := make(map[string]any, len(src))
	for k, v := range src {
		clone[k] = v
	}
	return clone
}

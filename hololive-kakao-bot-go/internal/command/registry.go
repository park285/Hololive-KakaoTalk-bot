package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
)

// ErrUnknownCommand is returned when a command dispatch is attempted for an
// unregistered key.
var ErrUnknownCommand = errors.New("unknown command")

// Registry stores command handlers keyed by their canonical names.
type Registry struct {
	mu        sync.RWMutex
	handlers  map[string]Command
	aliasKeys map[string]string
}

// NewRegistry constructs an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers:  make(map[string]Command),
		aliasKeys: make(map[string]string),
	}
}

// Register adds a command handler to the registry. The handler name is stored
// in lowercase form to provide case-insensitive lookups.
func (r *Registry) Register(handler Command) {
	if handler == nil {
		return
	}

	name := strings.ToLower(handler.Name())

	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[name] = handler
}

// Execute runs the handler registered for the provided key. Keys are compared in
// lowercase to maintain parity with Register behaviour.
func (r *Registry) Execute(ctx context.Context, cmdCtx *domain.CommandContext, key string, params map[string]any) error {
	if r == nil {
		return fmt.Errorf("command registry is nil")
	}

	handler := r.getHandler(key)
	if handler == nil {
		return fmt.Errorf("%w: %s", ErrUnknownCommand, key)
	}

	return handler.Execute(ctx, cmdCtx, params)
}

// Count returns the number of registered command handlers.
func (r *Registry) Count() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.handlers)
}

func (r *Registry) getHandler(key string) Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if key == "" {
		return nil
	}
	if handler, ok := r.handlers[strings.ToLower(key)]; ok {
		return handler
	}
	return nil
}

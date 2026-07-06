package framework

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ShutdownHookFunc is a function that performs cleanup during shutdown.
type ShutdownHookFunc func(ctx context.Context) error

// OrchestrationHook represents a named shutdown hook with a cleanup function.
type OrchestrationHook struct {
	Name string
	Func ShutdownHookFunc
}

// ShutdownOrchestrator manages the graceful shutdown of service components.
type ShutdownOrchestrator struct {
	hooks   []OrchestrationHook
	timeout time.Duration
	mu      sync.RWMutex
}

// NewShutdownOrchestrator creates a new shutdown orchestrator with the given timeout.
func NewShutdownOrchestrator(timeout time.Duration) *ShutdownOrchestrator {
	return &ShutdownOrchestrator{
		hooks:   make([]OrchestrationHook, 0),
		timeout: timeout,
	}
}

// RegisterHook adds a shutdown hook to be executed during shutdown.
// Hooks are executed in the order they were registered.
func (so *ShutdownOrchestrator) RegisterHook(name string, hookFunc ShutdownHookFunc) {
	so.mu.Lock()
	defer so.mu.Unlock()

	so.hooks = append(so.hooks, OrchestrationHook{
		Name: name,
		Func: hookFunc,
	})
}

// RegisterHookWithTimeout adds a shutdown hook with a specific timeout.
func (so *ShutdownOrchestrator) RegisterHookWithTimeout(name string, timeout time.Duration, hookFunc ShutdownHookFunc) {
	so.RegisterHook(name, func(ctx context.Context) error {
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return hookFunc(timeoutCtx)
	})
}

// Shutdown executes all registered hooks in order and waits for completion.
// Returns an error if any hook fails or if the overall timeout is exceeded.
func (so *ShutdownOrchestrator) Shutdown(ctx context.Context) error {
	so.mu.RLock()
	hooks := make([]OrchestrationHook, len(so.hooks))
	copy(hooks, so.hooks)
	so.mu.RUnlock()

	if len(hooks) == 0 {
		return nil
	}

	// Create a timeout context for the entire shutdown process
	timeoutCtx, cancel := context.WithTimeout(ctx, so.timeout)
	defer cancel()

	var errs []error
	var skipped []string
	timedOut := false

	// Execute hooks sequentially to maintain order
	for i := len(hooks) - 1; i >= 0; i-- { // Reverse order for proper cleanup
		hook := hooks[i]

		if !timedOut {
			select {
			case <-timeoutCtx.Done():
				timedOut = true
			default:
				// Continue with hook execution
			}
		}

		if timedOut {
			skipped = append(skipped, hook.Name)
			continue
		}

		if err := so.executeHook(timeoutCtx, hook); err != nil {
			errs = append(errs, fmt.Errorf("hook '%s' failed: %w", hook.Name, err))
		}
	}

	if len(skipped) > 0 {
		errs = append(errs, fmt.Errorf("shutdown timeout exceeded before executing hooks: %v", skipped))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// executeHook executes a single shutdown hook with proper error handling.
func (so *ShutdownOrchestrator) executeHook(ctx context.Context, hook OrchestrationHook) error {
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panic in shutdown hook '%s': %v", hook.Name, r)
			}
		}()
		done <- hook.Func(ctx)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("hook '%s' timed out", hook.Name)
	}
}

// ComponentShutdownHook creates a shutdown hook for a Component.
func ComponentShutdownHook(name string, component Component) OrchestrationHook {
	return OrchestrationHook{
		Name: fmt.Sprintf("component-%s", name),
		Func: func(ctx context.Context) error {
			return component.Stop(ctx)
		},
	}
}

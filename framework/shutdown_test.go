package framework

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestShutdownOrchestrator_NoHooks(t *testing.T) {
	so := NewShutdownOrchestrator(5 * time.Second)
	ctx := context.Background()
	if err := so.Shutdown(ctx); err != nil {
		t.Errorf("expected no error with no hooks, got: %v", err)
	}
}

func TestShutdownOrchestrator_SingleHook(t *testing.T) {
	so := NewShutdownOrchestrator(5 * time.Second)
	called := false
	so.RegisterHook("test", func(ctx context.Context) error {
		called = true
		return nil
	})

	if err := so.Shutdown(context.Background()); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if !called {
		t.Error("expected hook to be called")
	}
}

func TestShutdownOrchestrator_ReverseOrder(t *testing.T) {
	so := NewShutdownOrchestrator(5 * time.Second)
	order := []string{}

	so.RegisterHook("first", func(ctx context.Context) error {
		order = append(order, "first")
		return nil
	})
	so.RegisterHook("second", func(ctx context.Context) error {
		order = append(order, "second")
		return nil
	})
	so.RegisterHook("third", func(ctx context.Context) error {
		order = append(order, "third")
		return nil
	})

	if err := so.Shutdown(context.Background()); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Hooks execute in reverse registration order
	expected := []string{"third", "second", "first"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d hooks to run, got %d", len(expected), len(order))
	}
	for i, name := range expected {
		if order[i] != name {
			t.Errorf("position %d: expected %q, got %q", i, name, order[i])
		}
	}
}

func TestShutdownOrchestrator_HookError(t *testing.T) {
	so := NewShutdownOrchestrator(5 * time.Second)
	so.RegisterHook("failing", func(ctx context.Context) error {
		return errors.New("hook failed")
	})

	err := so.Shutdown(context.Background())
	if err == nil {
		t.Error("expected error from failing hook, got nil")
	}
}

func TestShutdownOrchestrator_MultipleHookErrors(t *testing.T) {
	so := NewShutdownOrchestrator(5 * time.Second)
	so.RegisterHook("hook1", func(ctx context.Context) error {
		return errors.New("error1")
	})
	so.RegisterHook("hook2", func(ctx context.Context) error {
		return errors.New("error2")
	})

	err := so.Shutdown(context.Background())
	if err == nil {
		t.Error("expected error from failing hooks, got nil")
	}
}

func TestShutdownOrchestrator_PanicRecovery(t *testing.T) {
	so := NewShutdownOrchestrator(5 * time.Second)
	so.RegisterHook("panicking", func(ctx context.Context) error {
		panic("something went wrong")
	})

	err := so.Shutdown(context.Background())
	if err == nil {
		t.Error("expected error from panicking hook, got nil")
	}
}

func TestShutdownOrchestrator_RegisterHookWithTimeout(t *testing.T) {
	so := NewShutdownOrchestrator(5 * time.Second)
	called := false

	so.RegisterHookWithTimeout("timed", 2*time.Second, func(ctx context.Context) error {
		called = true
		return nil
	})

	if err := so.Shutdown(context.Background()); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if !called {
		t.Error("expected hook to be called")
	}
}

func TestShutdownOrchestrator_HookTimeout(t *testing.T) {
	so := NewShutdownOrchestrator(100 * time.Millisecond)
	so.RegisterHook("slow", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	})

	err := so.Shutdown(context.Background())
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

func TestShutdownOrchestrator_ContextAlreadyCancelled(t *testing.T) {
	so := NewShutdownOrchestrator(5 * time.Second)
	so.RegisterHook("hook1", func(ctx context.Context) error { return nil })
	so.RegisterHook("hook2", func(ctx context.Context) error { return nil })

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// With context already cancelled, the overall timeout context also immediately expires.
	// Hooks may or may not run (race), but shutdown should not hang.
	_ = so.Shutdown(ctx) // just verify it returns
}

func TestComponentShutdownHook(t *testing.T) {
	comp := &TestComponent{}
	hook := ComponentShutdownHook("my-component", comp)

	if hook.Name != "component-my-component" {
		t.Errorf("expected name 'component-my-component', got %q", hook.Name)
	}

	ctx := context.Background()
	if err := hook.Func(ctx); err != nil {
		t.Errorf("expected no error from hook, got: %v", err)
	}
	if !comp.stopped {
		t.Error("expected component to be stopped")
	}
}

func TestComponentShutdownHook_Error(t *testing.T) {
	comp := &TestComponent{stopError: fmt.Errorf("stop failed")}
	hook := ComponentShutdownHook("failing", comp)

	ctx := context.Background()
	if err := hook.Func(ctx); err == nil {
		t.Error("expected error from hook, got nil")
	}
}

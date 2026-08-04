package forwarder

import (
	"context"
	"testing"
	"time"

	"cursor/internal/runtime"
)

type idleTimeoutResolver struct {
	timeout time.Duration
}

func (resolver idleTimeoutResolver) SelectChannelForModel(context.Context, string) (*runtime.ResolvedChannel, error) {
	return nil, nil
}

func (resolver idleTimeoutResolver) ProviderStreamIdleTimeout(context.Context) time.Duration {
	return resolver.timeout
}

func TestResolveProviderStreamIdleTimeoutUsesResolverConfig(t *testing.T) {
	service := &Service{resolver: idleTimeoutResolver{timeout: 240 * time.Second}}
	if got := resolveProviderStreamIdleTimeout(service, context.Background()); got != 240*time.Second {
		t.Fatalf("resolveProviderStreamIdleTimeout() = %s, want 4m", got)
	}
}

func TestResolveProviderStreamIdleTimeoutFallsBackForInvalidValue(t *testing.T) {
	service := &Service{resolver: idleTimeoutResolver{timeout: 0}}
	if got := resolveProviderStreamIdleTimeout(service, context.Background()); got != defaultProviderStreamIdleTimeout {
		t.Fatalf("resolveProviderStreamIdleTimeout() = %s, want %s", got, defaultProviderStreamIdleTimeout)
	}
}

func TestResolveProviderStreamIdleTimeoutClampsBelowMinimum(t *testing.T) {
	service := &Service{resolver: idleTimeoutResolver{timeout: time.Second}}
	if got := resolveProviderStreamIdleTimeout(service, context.Background()); got != minProviderStreamIdleTimeout {
		t.Fatalf("resolveProviderStreamIdleTimeout() = %s, want %s", got, minProviderStreamIdleTimeout)
	}
}

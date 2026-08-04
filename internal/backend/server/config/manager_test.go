package config

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestProviderStreamIdleTimeoutUsesStoredConfig(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "config.yaml"), "")
	manager, err := NewManager(context.Background(), store)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	cfg := manager.Current()
	cfg.ProviderStreamIdleTimeout = 240
	if _, err := manager.Save(context.Background(), cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if got := manager.ProviderStreamIdleTimeout(context.Background()); got != 240*time.Second {
		t.Fatalf("ProviderStreamIdleTimeout() = %s, want 4m", got)
	}
}

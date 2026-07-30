package integration

import (
	"context"
	"path/filepath"
	"testing"

	"modary/core/config"
	"modary/internal/app"
)

func TestBootstrapPasswordRotationAndDelegatorRevocation(t *testing.T) {
	dataDir := t.TempDir()
	cfg := config.Runtime{
		DataDir: dataDir, DatabasePath: filepath.Join(dataDir, "modary.db"), ListenAddress: "127.0.0.1:0",
		DemoPassword: "first-password", AgentToken: "rotation-agent-token",
	}
	first, err := app.Bootstrap(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Identity.Login(context.Background(), "admin", cfg.DemoPassword); err != nil {
		t.Fatalf("initial password rejected: %v", err)
	}
	if _, err := first.Identity.ResolveAgentToken(context.Background(), cfg.AgentToken); err != nil {
		t.Fatalf("initial agent token rejected: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	cfg.DemoPassword = "rotated-password"
	second, err := app.Bootstrap(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := second.Identity.Login(context.Background(), "admin", "first-password"); err == nil {
		t.Fatal("old bootstrap password remained valid after rotation")
	}
	if _, err := second.Identity.Login(context.Background(), "admin", cfg.DemoPassword); err != nil {
		t.Fatalf("rotated password rejected: %v", err)
	}
	if _, err := second.DB.ExecContext(context.Background(), `UPDATE modary_user SET active = 0 WHERE user_id = 'user_operator'`); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Identity.ResolveAgentToken(context.Background(), cfg.AgentToken); err == nil {
		t.Fatal("agent grant remained valid after its delegator was deactivated")
	}
}

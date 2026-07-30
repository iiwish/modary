package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultDemoPassword = "modary-demo"
	DefaultAgentToken   = "modary-agent-demo-token"
)

type Runtime struct {
	DataDir       string
	DatabasePath  string
	ListenAddress string
	DemoPassword  string
	AgentToken    string
}

func FromEnvironment() Runtime {
	dataDir := valueOrDefault("MODARY_DATA_DIR", "data")
	return Runtime{
		DataDir:       dataDir,
		DatabasePath:  valueOrDefault("MODARY_DATABASE_PATH", filepath.Join(dataDir, "modary.db")),
		ListenAddress: valueOrDefault("MODARY_LISTEN_ADDRESS", "127.0.0.1:8080"),
		DemoPassword:  valueOrDefault("MODARY_DEMO_PASSWORD", DefaultDemoPassword),
		AgentToken:    valueOrDefault("MODARY_AGENT_TOKEN", DefaultAgentToken),
	}
}

func ValidateForServe(runtime Runtime) error {
	host, _, err := net.SplitHostPort(runtime.ListenAddress)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", runtime.ListenAddress, err)
	}
	if isLoopbackHost(host) {
		return nil
	}
	if runtime.DemoPassword == DefaultDemoPassword || runtime.AgentToken == DefaultAgentToken {
		return fmt.Errorf("refusing non-loopback listen with demo credentials; set MODARY_DEMO_PASSWORD and MODARY_AGENT_TOKEN")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func valueOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

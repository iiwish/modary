package config

import "testing"

func TestValidateForServeRejectsDemoCredentialsOffLoopback(t *testing.T) {
	runtime := Runtime{ListenAddress: "0.0.0.0:8080", DemoPassword: DefaultDemoPassword, AgentToken: DefaultAgentToken}
	if err := ValidateForServe(runtime); err == nil {
		t.Fatal("expected demo credentials to be rejected for a non-loopback listener")
	}
	runtime.DemoPassword = "deployment-password"
	runtime.AgentToken = "deployment-agent-token"
	if err := ValidateForServe(runtime); err != nil {
		t.Fatalf("custom credentials rejected: %v", err)
	}
}

func TestValidateForServeAllowsLoopbackDevelopment(t *testing.T) {
	runtime := Runtime{ListenAddress: "127.0.0.1:8080", DemoPassword: DefaultDemoPassword, AgentToken: DefaultAgentToken}
	if err := ValidateForServe(runtime); err != nil {
		t.Fatalf("loopback development rejected: %v", err)
	}
}

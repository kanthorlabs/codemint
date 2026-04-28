package orchestrator

import (
	"context"
	"testing"

	"codemint.kanthorlabs.com/internal/config"
	"codemint.kanthorlabs.com/internal/domain"
)

// mockAgentRepoForHooks implements repository.AgentRepository for hook tests.
type mockAgentRepoForHooks struct {
	agents map[string]*domain.Agent
}

func (m *mockAgentRepoForHooks) FindByName(_ context.Context, name string) (*domain.Agent, error) {
	if m.agents == nil {
		return nil, nil
	}
	return m.agents[name], nil
}

func (m *mockAgentRepoForHooks) EnsureSystemAgents(_ context.Context) error {
	return nil
}

func TestPreStartupHook_ValidConfig(t *testing.T) {
	cfg := &config.Config{
		Assistants: map[string]config.AssistantConfig{
			"sys-default": {Provider: "opencode", Model: "gpt-4"},
		},
	}

	agentRepo := &mockAgentRepoForHooks{
		agents: map[string]*domain.Agent{
			"sys-default": {
				ID:   "agent-sys-default",
				Name: "sys-default",
				Type: domain.AgentTypeAssistant,
			},
		},
	}

	knownProviders := map[string]bool{"opencode": true, "codex": true}

	err := PreStartupHook(context.Background(), PreStartupHookConfig{
		Config:         cfg,
		AgentRepo:      agentRepo,
		KnownProviders: knownProviders,
	})

	if err != nil {
		t.Errorf("PreStartupHook returned error for valid config: %v", err)
	}
}

func TestPreStartupHook_MissingSysDefaultProvider(t *testing.T) {
	cfg := &config.Config{
		Assistants: map[string]config.AssistantConfig{
			"sys-default": {Provider: ""}, // Missing provider
		},
	}

	agentRepo := &mockAgentRepoForHooks{
		agents: map[string]*domain.Agent{
			"sys-default": {
				ID:   "agent-sys-default",
				Name: "sys-default",
				Type: domain.AgentTypeAssistant,
			},
		},
	}

	knownProviders := map[string]bool{"opencode": true}

	err := PreStartupHook(context.Background(), PreStartupHookConfig{
		Config:         cfg,
		AgentRepo:      agentRepo,
		KnownProviders: knownProviders,
	})

	if err == nil {
		t.Fatal("expected error for missing sys-default provider, got nil")
	}
}

func TestPreStartupHook_UnknownProvider(t *testing.T) {
	cfg := &config.Config{
		Assistants: map[string]config.AssistantConfig{
			"sys-default": {Provider: "unknown-provider"},
		},
	}

	agentRepo := &mockAgentRepoForHooks{
		agents: map[string]*domain.Agent{
			"sys-default": {
				ID:   "agent-sys-default",
				Name: "sys-default",
				Type: domain.AgentTypeAssistant,
			},
		},
	}

	knownProviders := map[string]bool{"opencode": true}

	err := PreStartupHook(context.Background(), PreStartupHookConfig{
		Config:         cfg,
		AgentRepo:      agentRepo,
		KnownProviders: knownProviders,
	})

	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}

func TestPreStartupHook_MissingSysDefaultAgent(t *testing.T) {
	cfg := &config.Config{
		Assistants: map[string]config.AssistantConfig{
			"sys-default": {Provider: "opencode"},
		},
	}

	// Agent repo with no sys-default agent
	agentRepo := &mockAgentRepoForHooks{
		agents: map[string]*domain.Agent{},
	}

	knownProviders := map[string]bool{"opencode": true}

	err := PreStartupHook(context.Background(), PreStartupHookConfig{
		Config:         cfg,
		AgentRepo:      agentRepo,
		KnownProviders: knownProviders,
	})

	if err == nil {
		t.Fatal("expected error for missing sys-default agent, got nil")
	}
}

func TestPreExitHook_NoError(t *testing.T) {
	err := PreExitHook(context.Background(), PreExitHookConfig{})
	if err != nil {
		t.Errorf("PreExitHook returned error: %v", err)
	}
}

func TestEnsureSysDefaultAgent_Valid(t *testing.T) {
	agentRepo := &mockAgentRepoForHooks{
		agents: map[string]*domain.Agent{
			"sys-default": {
				ID:   "agent-sys-default",
				Name: "sys-default",
				Type: domain.AgentTypeAssistant,
			},
		},
	}

	agent, err := EnsureSysDefaultAgent(context.Background(), agentRepo)
	if err != nil {
		t.Fatalf("EnsureSysDefaultAgent returned error: %v", err)
	}
	if agent == nil {
		t.Fatal("expected agent, got nil")
	}
	if agent.ID != "agent-sys-default" {
		t.Errorf("agent.ID = %q; want %q", agent.ID, "agent-sys-default")
	}
}

func TestEnsureSysDefaultAgent_NotSeeded(t *testing.T) {
	agentRepo := &mockAgentRepoForHooks{
		agents: map[string]*domain.Agent{},
	}

	_, err := EnsureSysDefaultAgent(context.Background(), agentRepo)
	if err == nil {
		t.Fatal("expected error for missing sys-default agent, got nil")
	}
}

func TestEnsureSysDefaultAgent_WrongType(t *testing.T) {
	agentRepo := &mockAgentRepoForHooks{
		agents: map[string]*domain.Agent{
			"sys-default": {
				ID:   "agent-sys-default",
				Name: "sys-default",
				Type: domain.AgentTypeHuman, // Wrong type
			},
		},
	}

	_, err := EnsureSysDefaultAgent(context.Background(), agentRepo)
	if err == nil {
		t.Fatal("expected error for wrong agent type, got nil")
	}
}

package config

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLoad_ValidYAML(t *testing.T) {
	content := `
workflows:
  - type: 0
    name: "Project Coding"
    description: "Coding tasks"
    triggers: ["implement", "fix"]
  - type: 1
    name: "Communication"
    description: "Inquiries"
`
	tmpFile := writeTestFile(t, content)

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if len(cfg.Workflows) != 2 {
		t.Errorf("expected 2 workflows, got %d", len(cfg.Workflows))
	}

	if cfg.Workflows[0].Name != "Project Coding" {
		t.Errorf("workflow[0].Name = %q, want 'Project Coding'", cfg.Workflows[0].Name)
	}

	if len(cfg.Workflows[0].Triggers) != 2 {
		t.Errorf("workflow[0].Triggers = %v, want 2 items", cfg.Workflows[0].Triggers)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	content := `
workflows:
  - type: invalid_not_a_number
    name: "Bad"
`
	tmpFile := writeTestFile(t, content)

	_, err := Load(tmpFile)
	if err == nil {
		t.Fatal("expected parse error for invalid YAML, got nil")
	}

	if !strings.Contains(err.Error(), "parse yaml") {
		t.Errorf("error should mention parse yaml: %v", err)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}

	if cfg == nil {
		t.Fatal("expected empty config, got nil")
	}

	if len(cfg.Workflows) != 0 {
		t.Errorf("expected 0 workflows for missing file, got %d", len(cfg.Workflows))
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	tmpFile := writeTestFile(t, "")

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if len(cfg.Workflows) != 0 {
		t.Errorf("expected 0 workflows for empty file, got %d", len(cfg.Workflows))
	}
}

// writeTestFile creates a temp file with the given content and returns its path.
func writeTestFile(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	return path
}

func TestLoad_LegacyAgentsKey_LogsDeprecation(t *testing.T) {
	content := `
workflows: []
agents:
  - name: foo
`
	tmpFile := writeTestFile(t, content)

	// Set up a test log handler to capture the warning.
	handler := &testLogHandler{}
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(oldLogger)

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}

	// Check that the deprecation warning was logged.
	if !handler.hasWarning("agents:", "deprecated") {
		t.Error("expected deprecation warning for 'agents:' key")
	}
}

func TestLoad_NoLegacyAgentsKey_NoWarning(t *testing.T) {
	content := `
workflows: []
assistants:
  sys-default:
    provider: opencode
`
	tmpFile := writeTestFile(t, content)

	handler := &testLogHandler{}
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(oldLogger)

	_, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if handler.hasWarning("agents:", "deprecated") {
		t.Error("should not log deprecation warning when agents: key is absent")
	}
}

// testLogHandler is a slog.Handler that records log records for testing.
type testLogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *testLogHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (h *testLogHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *testLogHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *testLogHandler) WithGroup(_ string) slog.Handler {
	return h
}

func (h *testLogHandler) hasWarning(keywords ...string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.Level == slog.LevelWarn {
			msg := r.Message
			allFound := true
			for _, kw := range keywords {
				if !strings.Contains(msg, kw) {
					allFound = false
					break
				}
			}
			if allFound {
				return true
			}
		}
	}
	return false
}

func TestLoad_AssistantsMap(t *testing.T) {
	content := `
workflows: []
assistants:
  sys-default:
    provider: opencode
    model: gpt-4
  brainstormer:
    provider: codex
`
	tmpFile := writeTestFile(t, content)

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load returned error for valid config: %v", err)
	}

	sysDefault := cfg.GetAssistant("sys-default")
	if sysDefault.Provider != "opencode" {
		t.Errorf("sys-default.Provider = %q; want %q", sysDefault.Provider, "opencode")
	}
	if sysDefault.Model != "gpt-4" {
		t.Errorf("sys-default.Model = %q; want %q", sysDefault.Model, "gpt-4")
	}

	brainstormer := cfg.GetAssistant("brainstormer")
	if brainstormer.Provider != "codex" {
		t.Errorf("brainstormer.Provider = %q; want %q", brainstormer.Provider, "codex")
	}
}

// TestExampleConfig_Loads verifies that the example config file loads without errors.
func TestExampleConfig_Loads(t *testing.T) {
	cfg, err := Load("../../configs/config.yaml.example")
	if err != nil {
		t.Fatalf("Load returned error for example config: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	// Verify some expected content.
	if len(cfg.Workflows) != 3 {
		t.Errorf("expected 3 workflows in example config, got %d", len(cfg.Workflows))
	}
}

// TestGetSysCoding_Configured returns sys-coding when configured.
func TestGetSysCoding_Configured(t *testing.T) {
	content := `
workflows: []
assistants:
  sys-default:
    provider: opencode
  sys-coding:
    provider: codex
    model: gpt-4
`
	tmpFile := writeTestFile(t, content)

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	sysCoding := cfg.GetSysCoding()
	if sysCoding.Provider != "codex" {
		t.Errorf("GetSysCoding().Provider = %q; want %q", sysCoding.Provider, "codex")
	}
	if sysCoding.Model != "gpt-4" {
		t.Errorf("GetSysCoding().Model = %q; want %q", sysCoding.Model, "gpt-4")
	}
}

// TestGetSysCoding_FallbackToSysDefault falls back to sys-default when sys-coding not configured.
func TestGetSysCoding_FallbackToSysDefault(t *testing.T) {
	content := `
workflows: []
assistants:
  sys-default:
    provider: opencode
    model: sonnet
`
	tmpFile := writeTestFile(t, content)

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	sysCoding := cfg.GetSysCoding()
	if sysCoding.Provider != "opencode" {
		t.Errorf("GetSysCoding().Provider = %q; want %q (fallback to sys-default)", sysCoding.Provider, "opencode")
	}
	if sysCoding.Model != "sonnet" {
		t.Errorf("GetSysCoding().Model = %q; want %q (fallback to sys-default)", sysCoding.Model, "sonnet")
	}
}

// TestGetSysCoding_EmptyConfig returns empty when nothing configured.
func TestGetSysCoding_EmptyConfig(t *testing.T) {
	content := `
workflows: []
`
	tmpFile := writeTestFile(t, content)

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	sysCoding := cfg.GetSysCoding()
	if sysCoding.Provider != "" {
		t.Errorf("GetSysCoding().Provider = %q; want empty string for unconfigured", sysCoding.Provider)
	}
}

package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
)

func TestWorkflowParser_Parse(t *testing.T) {
	t.Run("valid workflow", func(t *testing.T) {
		// Create temp directory matching workflow name
		tmpDir := t.TempDir()
		workflowDir := filepath.Join(tmpDir, "brainstorming")
		if err := os.Mkdir(workflowDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		content := `name: brainstorming
version: "1.0"
description: |
  5-phase brainstorming pipeline for feature planning.

settings:
  default_timeout: 3600000
  guardrails:
    verification: true
    confirmation: true
    retrospective: true

epics:
  - id: planning
    name: "Feature Planning"
    description: "Gather context, clarify requirements, generate tasks"
    stories:
      - id: gather
        name: "Context Intake"
        skill: "@codemint/gatherer"
      - id: clarify
        name: "Specification Clarification"
        skill: "@codemint/spec-writer"
        exit_on:
          command: "/generate"
      - id: generate
        name: "Task Generation"
        skill: "@codemint/task-generator"
        output:
          schema: "skills/task-generator/references/task-schema.json"
          handler: "create_implementation_tasks"
`
		workflowPath := filepath.Join(workflowDir, "WORKFLOW.yaml")
		if err := os.WriteFile(workflowPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		parser := NewWorkflowParser()
		wf, err := parser.Parse(workflowDir)
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}

		// Verify parsed content
		if wf.Name != "brainstorming" {
			t.Errorf("Name = %q, want %q", wf.Name, "brainstorming")
		}
		if wf.Version != "1.0" {
			t.Errorf("Version = %q, want %q", wf.Version, "1.0")
		}
		if wf.Settings.DefaultTimeout != 3600000 {
			t.Errorf("DefaultTimeout = %d, want %d", wf.Settings.DefaultTimeout, 3600000)
		}
		if !wf.Settings.Guardrails.Verification {
			t.Error("Guardrails.Verification = false, want true")
		}
		if len(wf.Epics) != 1 {
			t.Fatalf("len(Epics) = %d, want %d", len(wf.Epics), 1)
		}
		if wf.Epics[0].ID != "planning" {
			t.Errorf("Epics[0].ID = %q, want %q", wf.Epics[0].ID, "planning")
		}
		if len(wf.Epics[0].Stories) != 3 {
			t.Fatalf("len(Stories) = %d, want %d", len(wf.Epics[0].Stories), 3)
		}

		// Verify first story
		story := wf.Epics[0].Stories[0]
		if story.ID != "gather" {
			t.Errorf("Stories[0].ID = %q, want %q", story.ID, "gather")
		}
		if story.Skill != "@codemint/gatherer" {
			t.Errorf("Stories[0].Skill = %q, want %q", story.Skill, "@codemint/gatherer")
		}

		// Verify story with exit_on
		story2 := wf.Epics[0].Stories[1]
		if story2.ExitOn == nil {
			t.Fatal("Stories[1].ExitOn = nil, want non-nil")
		}
		if story2.ExitOn.Command != "/generate" {
			t.Errorf("Stories[1].ExitOn.Command = %q, want %q", story2.ExitOn.Command, "/generate")
		}

		// Verify story with output
		story3 := wf.Epics[0].Stories[2]
		if story3.Output == nil {
			t.Fatal("Stories[2].Output = nil, want non-nil")
		}
		if story3.Output.Handler != "create_implementation_tasks" {
			t.Errorf("Stories[2].Output.Handler = %q, want %q", story3.Output.Handler, "create_implementation_tasks")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		tmpDir := t.TempDir()
		workflowDir := filepath.Join(tmpDir, "test")
		if err := os.Mkdir(workflowDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		content := `version: "1.0"
epics:
  - id: e1
    name: "Epic 1"
`
		workflowPath := filepath.Join(workflowDir, "WORKFLOW.yaml")
		if err := os.WriteFile(workflowPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		parser := NewWorkflowParser()
		_, err := parser.Parse(workflowDir)
		if err == nil {
			t.Fatal("Parse() expected error, got nil")
		}
		if err != ErrMissingName {
			t.Errorf("Parse() error = %v, want %v", err, ErrMissingName)
		}
	})

	t.Run("missing version", func(t *testing.T) {
		tmpDir := t.TempDir()
		workflowDir := filepath.Join(tmpDir, "test")
		if err := os.Mkdir(workflowDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		content := `name: test
epics:
  - id: e1
    name: "Epic 1"
`
		workflowPath := filepath.Join(workflowDir, "WORKFLOW.yaml")
		if err := os.WriteFile(workflowPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		parser := NewWorkflowParser()
		_, err := parser.Parse(workflowDir)
		if err == nil {
			t.Fatal("Parse() expected error, got nil")
		}
		if err != ErrMissingVersion {
			t.Errorf("Parse() error = %v, want %v", err, ErrMissingVersion)
		}
	})

	t.Run("missing epics", func(t *testing.T) {
		tmpDir := t.TempDir()
		workflowDir := filepath.Join(tmpDir, "test")
		if err := os.Mkdir(workflowDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		content := `name: test
version: "1.0"
`
		workflowPath := filepath.Join(workflowDir, "WORKFLOW.yaml")
		if err := os.WriteFile(workflowPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		parser := NewWorkflowParser()
		_, err := parser.Parse(workflowDir)
		if err == nil {
			t.Fatal("Parse() expected error, got nil")
		}
		if err != ErrMissingEpics {
			t.Errorf("Parse() error = %v, want %v", err, ErrMissingEpics)
		}
	})

	t.Run("name mismatch", func(t *testing.T) {
		tmpDir := t.TempDir()
		workflowDir := filepath.Join(tmpDir, "myworkflow")
		if err := os.Mkdir(workflowDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		content := `name: different-name
version: "1.0"
epics:
  - id: e1
    name: "Epic 1"
`
		workflowPath := filepath.Join(workflowDir, "WORKFLOW.yaml")
		if err := os.WriteFile(workflowPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		parser := NewWorkflowParser()
		_, err := parser.Parse(workflowDir)
		if err == nil {
			t.Fatal("Parse() expected error, got nil")
		}
	})

	t.Run("epic missing id", func(t *testing.T) {
		tmpDir := t.TempDir()
		workflowDir := filepath.Join(tmpDir, "test")
		if err := os.Mkdir(workflowDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		content := `name: test
version: "1.0"
epics:
  - name: "Epic Without ID"
`
		workflowPath := filepath.Join(workflowDir, "WORKFLOW.yaml")
		if err := os.WriteFile(workflowPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		parser := NewWorkflowParser()
		_, err := parser.Parse(workflowDir)
		if err == nil {
			t.Fatal("Parse() expected error, got nil")
		}
	})

	t.Run("story missing id", func(t *testing.T) {
		tmpDir := t.TempDir()
		workflowDir := filepath.Join(tmpDir, "test")
		if err := os.Mkdir(workflowDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		content := `name: test
version: "1.0"
epics:
  - id: e1
    name: "Epic 1"
    stories:
      - name: "Story Without ID"
`
		workflowPath := filepath.Join(workflowDir, "WORKFLOW.yaml")
		if err := os.WriteFile(workflowPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		parser := NewWorkflowParser()
		_, err := parser.Parse(workflowDir)
		if err == nil {
			t.Fatal("Parse() expected error, got nil")
		}
	})

	t.Run("default settings", func(t *testing.T) {
		tmpDir := t.TempDir()
		workflowDir := filepath.Join(tmpDir, "minimal")
		if err := os.Mkdir(workflowDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		content := `name: minimal
version: "1.0"
epics:
  - id: e1
    name: "Epic 1"
    stories:
      - id: s1
        name: "Story 1"
`
		workflowPath := filepath.Join(workflowDir, "WORKFLOW.yaml")
		if err := os.WriteFile(workflowPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		parser := NewWorkflowParser()
		wf, err := parser.Parse(workflowDir)
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}

		// Verify default settings
		if wf.Settings.DefaultTimeout != domain.DefaultTaskTimeout {
			t.Errorf("DefaultTimeout = %d, want %d", wf.Settings.DefaultTimeout, domain.DefaultTaskTimeout)
		}
		if !wf.Settings.Guardrails.Verification {
			t.Error("Guardrails.Verification = false, want true")
		}
		if !wf.Settings.Guardrails.Confirmation {
			t.Error("Guardrails.Confirmation = false, want true")
		}
		if !wf.Settings.Guardrails.Retrospective {
			t.Error("Guardrails.Retrospective = false, want true")
		}
	})

	t.Run("routes parsing", func(t *testing.T) {
		tmpDir := t.TempDir()
		workflowDir := filepath.Join(tmpDir, "routing")
		if err := os.Mkdir(workflowDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		content := `name: routing
version: "1.0"
epics:
  - id: e1
    name: "Epic 1"
    stories:
      - id: s1
        name: "Story 1"
        routes:
          success: "s2"
          failure: "s3"
      - id: s2
        name: "Story 2 (success path)"
      - id: s3
        name: "Story 3 (failure path)"
`
		workflowPath := filepath.Join(workflowDir, "WORKFLOW.yaml")
		if err := os.WriteFile(workflowPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		parser := NewWorkflowParser()
		wf, err := parser.Parse(workflowDir)
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}

		story := wf.Epics[0].Stories[0]
		if len(story.Routes) != 2 {
			t.Fatalf("len(Routes) = %d, want %d", len(story.Routes), 2)
		}
		if story.Routes[domain.TaskStatusSuccess] != "s2" {
			t.Errorf("Routes[Success] = %q, want %q", story.Routes[domain.TaskStatusSuccess], "s2")
		}
		if story.Routes[domain.TaskStatusFailure] != "s3" {
			t.Errorf("Routes[Failure] = %q, want %q", story.Routes[domain.TaskStatusFailure], "s3")
		}
	})

	t.Run("task type parsing", func(t *testing.T) {
		tmpDir := t.TempDir()
		workflowDir := filepath.Join(tmpDir, "types")
		if err := os.Mkdir(workflowDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		content := `name: types
version: "1.0"
epics:
  - id: e1
    name: "Epic 1"
    stories:
      - id: s1
        name: "Coding Story"
        type: coding
      - id: s2
        name: "Verification Story"
        type: verification
      - id: s3
        name: "Confirmation Story"
        type: confirmation
      - id: s4
        name: "Coordination Story"
        type: coordination
      - id: s5
        name: "Retrospective Story"
        type: retrospective
`
		workflowPath := filepath.Join(workflowDir, "WORKFLOW.yaml")
		if err := os.WriteFile(workflowPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		parser := NewWorkflowParser()
		wf, err := parser.Parse(workflowDir)
		if err != nil {
			t.Fatalf("Parse() error: %v", err)
		}

		stories := wf.Epics[0].Stories
		if stories[0].Type != domain.TaskTypeCoding {
			t.Errorf("Stories[0].Type = %d, want %d", stories[0].Type, domain.TaskTypeCoding)
		}
		if stories[1].Type != domain.TaskTypeVerification {
			t.Errorf("Stories[1].Type = %d, want %d", stories[1].Type, domain.TaskTypeVerification)
		}
		if stories[2].Type != domain.TaskTypeConfirmation {
			t.Errorf("Stories[2].Type = %d, want %d", stories[2].Type, domain.TaskTypeConfirmation)
		}
		if stories[3].Type != domain.TaskTypeCoordination {
			t.Errorf("Stories[3].Type = %d, want %d", stories[3].Type, domain.TaskTypeCoordination)
		}
		if stories[4].Type != domain.TaskTypeRetrospective {
			t.Errorf("Stories[4].Type = %d, want %d", stories[4].Type, domain.TaskTypeRetrospective)
		}
	})
}

func TestDefaultGuardrailSettings(t *testing.T) {
	settings := domain.DefaultGuardrailSettings()

	if !settings.Verification {
		t.Error("Verification = false, want true")
	}
	if !settings.Confirmation {
		t.Error("Confirmation = false, want true")
	}
	if !settings.Retrospective {
		t.Error("Retrospective = false, want true")
	}
}

func TestDefaultWorkflowSettings(t *testing.T) {
	settings := domain.DefaultWorkflowSettings()

	if settings.DefaultTimeout != domain.DefaultTaskTimeout {
		t.Errorf("DefaultTimeout = %d, want %d", settings.DefaultTimeout, domain.DefaultTaskTimeout)
	}
	if !settings.Guardrails.Verification {
		t.Error("Guardrails.Verification = false, want true")
	}
}

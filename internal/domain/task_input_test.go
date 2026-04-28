package domain

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseTaskInput_RoundTrip(t *testing.T) {
	// Create a TaskInput with all fields populated.
	original := &TaskInput{
		Prompt:       "Implement the user login feature",
		ContextFiles: []string{"src/auth/login.go", "src/models/user.go"},
		Tools:        []string{"read", "write", "bash"},
		Command:      "go test ./...",
		Cwd:          "/project/root",
		Metadata: map[string]string{
			"story_id": "3.15",
			"priority": "high",
		},
		Skill: "@codemint/gatherer",
	}

	// Marshal to JSON.
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Parse back from JSON.
	parsed, err := ParseTaskInput(string(data))
	if err != nil {
		t.Fatalf("ParseTaskInput failed: %v", err)
	}

	// Verify all fields.
	if parsed.Prompt != original.Prompt {
		t.Errorf("Prompt mismatch: got %q, want %q", parsed.Prompt, original.Prompt)
	}

	if len(parsed.ContextFiles) != len(original.ContextFiles) {
		t.Errorf("ContextFiles length mismatch: got %d, want %d",
			len(parsed.ContextFiles), len(original.ContextFiles))
	}
	for i, file := range parsed.ContextFiles {
		if file != original.ContextFiles[i] {
			t.Errorf("ContextFiles[%d] mismatch: got %q, want %q",
				i, file, original.ContextFiles[i])
		}
	}

	if len(parsed.Tools) != len(original.Tools) {
		t.Errorf("Tools length mismatch: got %d, want %d",
			len(parsed.Tools), len(original.Tools))
	}
	for i, tool := range parsed.Tools {
		if tool != original.Tools[i] {
			t.Errorf("Tools[%d] mismatch: got %q, want %q",
				i, tool, original.Tools[i])
		}
	}

	if parsed.Command != original.Command {
		t.Errorf("Command mismatch: got %q, want %q", parsed.Command, original.Command)
	}

	if parsed.Cwd != original.Cwd {
		t.Errorf("Cwd mismatch: got %q, want %q", parsed.Cwd, original.Cwd)
	}

	if len(parsed.Metadata) != len(original.Metadata) {
		t.Errorf("Metadata length mismatch: got %d, want %d",
			len(parsed.Metadata), len(original.Metadata))
	}
	for key, val := range parsed.Metadata {
		if original.Metadata[key] != val {
			t.Errorf("Metadata[%q] mismatch: got %q, want %q",
				key, val, original.Metadata[key])
		}
	}

	if parsed.Skill != original.Skill {
		t.Errorf("Skill mismatch: got %q, want %q", parsed.Skill, original.Skill)
	}
}

func TestParseTaskInput_LegacyFallback(t *testing.T) {
	legacyText := "Please implement the user authentication feature with OAuth2 support"

	parsed, err := ParseTaskInput(legacyText)

	// Should return ErrLegacyText.
	if !errors.Is(err, ErrLegacyText) {
		t.Errorf("expected ErrLegacyText, got %v", err)
	}

	// Should still return a valid TaskInput with the text as Prompt.
	if parsed == nil {
		t.Fatal("expected non-nil TaskInput")
	}
	if parsed.Prompt != legacyText {
		t.Errorf("Prompt mismatch: got %q, want %q", parsed.Prompt, legacyText)
	}
}

func TestParseTaskInput_EmptyInput(t *testing.T) {
	parsed, err := ParseTaskInput("")

	if !errors.Is(err, ErrEmptyInput) {
		t.Errorf("expected ErrEmptyInput, got %v", err)
	}
	if parsed != nil {
		t.Errorf("expected nil TaskInput for empty input, got %+v", parsed)
	}
}

func TestParseTaskInput_MinimalJSON(t *testing.T) {
	input := `{"prompt": "Hello world"}`

	parsed, err := ParseTaskInput(input)
	if err != nil {
		t.Fatalf("ParseTaskInput failed: %v", err)
	}

	if parsed.Prompt != "Hello world" {
		t.Errorf("Prompt mismatch: got %q, want %q", parsed.Prompt, "Hello world")
	}
	if len(parsed.ContextFiles) != 0 {
		t.Errorf("expected empty ContextFiles, got %v", parsed.ContextFiles)
	}
	if len(parsed.Tools) != 0 {
		t.Errorf("expected empty Tools, got %v", parsed.Tools)
	}
	if parsed.Skill != "" {
		t.Errorf("expected empty Skill, got %q", parsed.Skill)
	}
}

func TestParseTaskInput_JSONWithSkill(t *testing.T) {
	input := `{
		"prompt": "Run gatherer pass",
		"skill": "@codemint/gatherer"
	}`

	parsed, err := ParseTaskInput(input)
	if err != nil {
		t.Fatalf("ParseTaskInput failed: %v", err)
	}

	if parsed.Prompt != "Run gatherer pass" {
		t.Errorf("Prompt mismatch: got %q", parsed.Prompt)
	}
	if parsed.Skill != "@codemint/gatherer" {
		t.Errorf("Skill mismatch: got %q, want %q", parsed.Skill, "@codemint/gatherer")
	}

	// Verify omitempty: marshal and check JSON doesn't have empty skill if not set
	noSkill := &TaskInput{Prompt: "Hello"}
	data, _ := json.Marshal(noSkill)
	if string(data) != `{"prompt":"Hello"}` {
		t.Errorf("Expected omitempty to exclude skill, got %s", string(data))
	}
}

func TestParseTaskInput_JSONWithContextFiles(t *testing.T) {
	input := `{
		"prompt": "Implement feature X",
		"context_files": ["src/main.go", "pkg/utils/helper.go"]
	}`

	parsed, err := ParseTaskInput(input)
	if err != nil {
		t.Fatalf("ParseTaskInput failed: %v", err)
	}

	if parsed.Prompt != "Implement feature X" {
		t.Errorf("Prompt mismatch: got %q", parsed.Prompt)
	}
	if len(parsed.ContextFiles) != 2 {
		t.Errorf("expected 2 context files, got %d", len(parsed.ContextFiles))
	}
	if parsed.ContextFiles[0] != "src/main.go" {
		t.Errorf("ContextFiles[0] mismatch: got %q", parsed.ContextFiles[0])
	}
	if parsed.ContextFiles[1] != "pkg/utils/helper.go" {
		t.Errorf("ContextFiles[1] mismatch: got %q", parsed.ContextFiles[1])
	}
}

func TestTaskInput_Validate(t *testing.T) {
	tests := []struct {
		name    string
		input   *TaskInput
		wantErr bool
	}{
		{
			name:    "valid with prompt only",
			input:   &TaskInput{Prompt: "Do something"},
			wantErr: false,
		},
		{
			name:    "valid with all fields",
			input:   &TaskInput{Prompt: "Do something", ContextFiles: []string{"a.go"}},
			wantErr: false,
		},
		{
			name:    "missing prompt",
			input:   &TaskInput{ContextFiles: []string{"a.go"}},
			wantErr: true,
		},
		{
			name:    "empty struct",
			input:   &TaskInput{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseTaskInput_InvalidJSON(t *testing.T) {
	// Invalid JSON that looks like it might be JSON but isn't.
	inputs := []string{
		`{"prompt": "missing closing brace"`,
		`{prompt: "no quotes on key"}`,
		`["array", "not", "object"]`,
	}

	for _, input := range inputs {
		parsed, err := ParseTaskInput(input)
		// Invalid JSON should be treated as legacy text.
		if !errors.Is(err, ErrLegacyText) {
			t.Errorf("input %q: expected ErrLegacyText, got %v", input, err)
		}
		if parsed == nil {
			t.Errorf("input %q: expected non-nil TaskInput", input)
		} else if parsed.Prompt != input {
			t.Errorf("input %q: Prompt mismatch: got %q", input, parsed.Prompt)
		}
	}
}

// --- Task 3.15.5: Contract Test with Fixture ---

// TestTaskInput_Fixture_Compatible verifies that the fixture file produced by
// the Task Generator (EPIC-02 §2.13) is compatible with the TaskInput schema.
// This test guards against silent schema drift between EPIC-02 and EPIC-03.
func TestTaskInput_Fixture_Compatible(t *testing.T) {
	// Get the path to testdata relative to this test file.
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get test file path")
	}
	testdataDir := filepath.Join(filepath.Dir(filename), "testdata")
	fixturePath := filepath.Join(testdataDir, "task_input_v1.json")

	// Read the fixture file.
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	// Parse the fixture.
	parsed, err := ParseTaskInput(string(data))
	if err != nil {
		t.Fatalf("ParseTaskInput failed on fixture: %v", err)
	}

	// Validate required fields.
	if err := parsed.Validate(); err != nil {
		t.Fatalf("fixture validation failed: %v", err)
	}

	// Verify all expected fields are recognized and populated.
	if parsed.Prompt == "" {
		t.Error("fixture: prompt should not be empty")
	}

	if len(parsed.ContextFiles) == 0 {
		t.Error("fixture: context_files should not be empty")
	}

	if len(parsed.Tools) == 0 {
		t.Error("fixture: tools should not be empty")
	}

	// Verify specific fixture content to detect unexpected changes.
	expectedFiles := []string{
		"src/auth/handler.go",
		"src/models/user.go",
		"src/middleware/auth.go",
	}
	for i, expected := range expectedFiles {
		if i >= len(parsed.ContextFiles) {
			t.Errorf("fixture: missing context_files[%d]: %s", i, expected)
			continue
		}
		if parsed.ContextFiles[i] != expected {
			t.Errorf("fixture: context_files[%d] mismatch: got %q, want %q",
				i, parsed.ContextFiles[i], expected)
		}
	}

	expectedTools := []string{"read", "write", "bash", "glob", "grep"}
	for i, expected := range expectedTools {
		if i >= len(parsed.Tools) {
			t.Errorf("fixture: missing tools[%d]: %s", i, expected)
			continue
		}
		if parsed.Tools[i] != expected {
			t.Errorf("fixture: tools[%d] mismatch: got %q, want %q",
				i, parsed.Tools[i], expected)
		}
	}

	// Verify optional fields.
	if parsed.Command != "go test ./src/auth/..." {
		t.Errorf("fixture: command mismatch: got %q", parsed.Command)
	}

	if parsed.Cwd != "/project/root" {
		t.Errorf("fixture: cwd mismatch: got %q", parsed.Cwd)
	}

	// Verify metadata.
	if parsed.Metadata == nil {
		t.Error("fixture: metadata should not be nil")
	} else {
		if parsed.Metadata["story_id"] != "2.13" {
			t.Errorf("fixture: metadata[story_id] mismatch: got %q", parsed.Metadata["story_id"])
		}
		if parsed.Metadata["epic_id"] != "02" {
			t.Errorf("fixture: metadata[epic_id] mismatch: got %q", parsed.Metadata["epic_id"])
		}
		if parsed.Metadata["generator_version"] != "1.0" {
			t.Errorf("fixture: metadata[generator_version] mismatch: got %q", parsed.Metadata["generator_version"])
		}
	}
}

// TestTaskInput_Fixture_RoundTrip verifies that the fixture can be serialized
// and deserialized without data loss.
func TestTaskInput_Fixture_RoundTrip(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get test file path")
	}
	testdataDir := filepath.Join(filepath.Dir(filename), "testdata")
	fixturePath := filepath.Join(testdataDir, "task_input_v1.json")

	// Read the fixture file.
	originalData, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	// Parse the fixture.
	parsed, err := ParseTaskInput(string(originalData))
	if err != nil {
		t.Fatalf("ParseTaskInput failed: %v", err)
	}

	// Marshal back to JSON.
	roundTripData, err := json.Marshal(parsed)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// Parse the round-trip data.
	reparsed, err := ParseTaskInput(string(roundTripData))
	if err != nil {
		t.Fatalf("second ParseTaskInput failed: %v", err)
	}

	// Compare the two parsed structures.
	if parsed.Prompt != reparsed.Prompt {
		t.Errorf("round-trip: Prompt mismatch")
	}

	if len(parsed.ContextFiles) != len(reparsed.ContextFiles) {
		t.Errorf("round-trip: ContextFiles length mismatch")
	}

	if len(parsed.Tools) != len(reparsed.Tools) {
		t.Errorf("round-trip: Tools length mismatch")
	}

	if parsed.Command != reparsed.Command {
		t.Errorf("round-trip: Command mismatch")
	}

	if parsed.Cwd != reparsed.Cwd {
		t.Errorf("round-trip: Cwd mismatch")
	}

	if len(parsed.Metadata) != len(reparsed.Metadata) {
		t.Errorf("round-trip: Metadata length mismatch")
	}
}

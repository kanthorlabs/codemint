package acp

import (
	"strings"
	"testing"
)

func TestBuildSystemPrompt(t *testing.T) {
	t.Run("empty memory includes only preamble", func(t *testing.T) {
		prompt := BuildSystemPrompt(HotMemory{})

		if !strings.Contains(prompt, "You are an execution agent invoked by CodeMint") {
			t.Error("prompt should contain preamble header")
		}

		if !strings.Contains(prompt, "HIERARCHY OF AUTHORITY") {
			t.Error("prompt should contain hierarchy of authority")
		}

		// Should not contain any section headers when memory is empty.
		if strings.Contains(prompt, "--- Project Memory: Preferences ---") {
			t.Error("empty preferences should not include preferences section")
		}
		if strings.Contains(prompt, "--- Project Memory: Architecture Decisions ---") {
			t.Error("empty decisions should not include decisions section")
		}
		if strings.Contains(prompt, "--- Project Memory: Known Bugs Index ---") {
			t.Error("empty bugs index should not include bugs section")
		}
	})

	t.Run("includes preferences when present", func(t *testing.T) {
		mem := HotMemory{
			Preferences: "Use tabs for indentation",
		}

		prompt := BuildSystemPrompt(mem)

		if !strings.Contains(prompt, "--- Project Memory: Preferences ---") {
			t.Error("prompt should include preferences header")
		}
		if !strings.Contains(prompt, "Use tabs for indentation") {
			t.Error("prompt should include preferences content")
		}

		// Other sections should not be present.
		if strings.Contains(prompt, "--- Project Memory: Architecture Decisions ---") {
			t.Error("empty decisions should not include decisions section")
		}
	})

	t.Run("includes decisions when present", func(t *testing.T) {
		mem := HotMemory{
			Decisions: "ADR-001: Use PostgreSQL for storage",
		}

		prompt := BuildSystemPrompt(mem)

		if !strings.Contains(prompt, "--- Project Memory: Architecture Decisions ---") {
			t.Error("prompt should include decisions header")
		}
		if !strings.Contains(prompt, "ADR-001: Use PostgreSQL") {
			t.Error("prompt should include decisions content")
		}
	})

	t.Run("includes bugs index when present", func(t *testing.T) {
		mem := HotMemory{
			BugsIndex: "BUG-001: Race condition in worker pool",
		}

		prompt := BuildSystemPrompt(mem)

		if !strings.Contains(prompt, "--- Project Memory: Known Bugs Index ---") {
			t.Error("prompt should include bugs index header")
		}
		if !strings.Contains(prompt, "BUG-001: Race condition") {
			t.Error("prompt should include bugs index content")
		}
	})

	t.Run("includes all sections when all present", func(t *testing.T) {
		mem := HotMemory{
			Preferences: "Prefer short functions",
			Decisions:   "Use dependency injection",
			BugsIndex:   "Known issue with timeouts",
		}

		prompt := BuildSystemPrompt(mem)

		if !strings.Contains(prompt, "--- Project Memory: Preferences ---") {
			t.Error("prompt should include preferences header")
		}
		if !strings.Contains(prompt, "--- Project Memory: Architecture Decisions ---") {
			t.Error("prompt should include decisions header")
		}
		if !strings.Contains(prompt, "--- Project Memory: Known Bugs Index ---") {
			t.Error("prompt should include bugs index header")
		}

		// Verify content order: preamble, preferences, decisions, bugs.
		prefsIdx := strings.Index(prompt, "Preferences ---")
		decisionsIdx := strings.Index(prompt, "Architecture Decisions ---")
		bugsIdx := strings.Index(prompt, "Known Bugs Index ---")

		if prefsIdx > decisionsIdx {
			t.Error("preferences should come before decisions")
		}
		if decisionsIdx > bugsIdx {
			t.Error("decisions should come before bugs index")
		}
	})

	t.Run("includes memory-override instruction", func(t *testing.T) {
		prompt := BuildSystemPrompt(HotMemory{})

		if !strings.Contains(prompt, "[memory-override]") {
			t.Error("prompt should instruct agent to emit memory-override tag")
		}
	})
}

func TestSystemPromptPreamble(t *testing.T) {
	// Verify the preamble content is correct.
	if !strings.Contains(systemPromptPreamble, "You are an execution agent") {
		t.Error("preamble should identify the agent role")
	}

	if !strings.Contains(systemPromptPreamble, "HIERARCHY OF AUTHORITY") {
		t.Error("preamble should establish hierarchy")
	}

	if !strings.Contains(systemPromptPreamble, "1. The Current Prompt") {
		t.Error("preamble should list current prompt as highest priority")
	}

	if !strings.Contains(systemPromptPreamble, "2. Project Memory") {
		t.Error("preamble should list project memory as second priority")
	}

	if !strings.Contains(systemPromptPreamble, "3. Global CodeMint Rules") {
		t.Error("preamble should list global rules as third priority")
	}
}

package acp

import (
	"strings"
)

// systemPromptPreamble is the header that establishes the Hierarchy of Authority.
const systemPromptPreamble = `You are an execution agent invoked by CodeMint.

HIERARCHY OF AUTHORITY (highest first):
  1. The Current Prompt / Living Spec.
  2. Project Memory (below).
  3. Global CodeMint Rules.

When you intentionally override a project preference from memory based on the current prompt,
emit a [memory-override] tag at the start of your response to signal this decision.
`

// BuildSystemPrompt constructs the agent system prompt with memory injection.
// It includes the Hierarchy of Authority preamble and any non-empty memory sections.
// Empty memory sections are omitted entirely (no heading shown).
func BuildSystemPrompt(mem HotMemory) string {
	var sb strings.Builder

	sb.WriteString(systemPromptPreamble)

	// Append non-empty memory sections.
	if mem.Preferences != "" {
		sb.WriteString("\n--- Project Memory: Preferences ---\n")
		sb.WriteString(mem.Preferences)
		sb.WriteString("\n")
	}

	if mem.Decisions != "" {
		sb.WriteString("\n--- Project Memory: Architecture Decisions ---\n")
		sb.WriteString(mem.Decisions)
		sb.WriteString("\n")
	}

	if mem.BugsIndex != "" {
		sb.WriteString("\n--- Project Memory: Known Bugs Index ---\n")
		sb.WriteString(mem.BugsIndex)
		sb.WriteString("\n")
	}

	return sb.String()
}

// BuildSystemPromptFromProjectID loads hot memory for the given project
// and constructs the system prompt. This is a convenience function that
// combines LoadHotMemory and BuildSystemPrompt.
func BuildSystemPromptFromProjectID(projectID string) (string, error) {
	mem, err := LoadHotMemory(projectID)
	if err != nil {
		return "", err
	}
	return BuildSystemPrompt(mem), nil
}

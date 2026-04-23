package orchestrator

import (
	"context"
	"errors"
	"fmt"
)

// ErrExtractionNotImplemented is returned by Extract when no real System
// Assistant backend has been wired up yet. It is a sentinel callers can
// check with errors.Is to detect the unimplemented state.
var ErrExtractionNotImplemented = errors.New("orchestrator: System Assistant extraction not implemented")

// Extractor uses the System Assistant to parse a raw natural-language argument
// string into a structured JSON representation expected by a command handler.
//
// This is the System Assistant Extractor described in task 1.6.5. The real
// implementation will create a background task assigned to the System Agent,
// wait for its output, and unmarshal the JSON into schema. For now it returns
// ErrExtractionNotImplemented so that command handlers can gracefully fall
// back to prompting the user for explicit flags.
type Extractor struct {
	// systemAssistant will be injected once the AI layer (EPIC-02) is wired up.
	systemAssistant func(ctx context.Context, prompt string) (string, error)
}

// NewExtractor creates an Extractor. systemAssistant may be nil; Extract will
// return ErrExtractionNotImplemented in that case.
func NewExtractor(systemAssistant func(ctx context.Context, prompt string) (string, error)) *Extractor {
	return &Extractor{systemAssistant: systemAssistant}
}

// Extract sends rawArgs to the System Assistant with a JSON-schema prompt and
// unmarshals the response into schema (must be a pointer). Returns
// ErrExtractionNotImplemented when no assistant is configured.
func (e *Extractor) Extract(ctx context.Context, rawArgs string, schema any) error {
	if e.systemAssistant == nil {
		return fmt.Errorf("%w: provide flags explicitly (e.g. -t \"title\" -p high)",
			ErrExtractionNotImplemented)
	}

	// Future implementation:
	//   1. Build a structured prompt asking the assistant to output JSON matching
	//      the schema type (derived via reflection or a caller-supplied hint).
	//   2. Call e.systemAssistant(ctx, prompt).
	//   3. json.Unmarshal the response into schema.
	//   4. Return any unmarshal or validation error.
	//
	// Placeholder until EPIC-02 assistant integration is complete.
	return fmt.Errorf("%w", ErrExtractionNotImplemented)
}

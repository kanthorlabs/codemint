// Package repl provides the Read-Eval-Print Loop functionality for CodeMint.
package repl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"codemint.kanthorlabs.com/internal/registry"
)

// Dispatcher defines the interface for routing user input to handlers.
// This interface allows the REPL loop to remain decoupled from the
// concrete orchestrator.Dispatcher implementation.
type Dispatcher interface {
	// DispatchInput parses and routes user input to the appropriate handler.
	// Returns registry.ErrShutdownGracefully when the user requests to exit.
	DispatchInput(ctx context.Context, input string) error
}

// Loop runs the main REPL loop, reading user input from stdin and dispatching
// it through the provided dispatcher. It exits when:
//   - A command returns registry.ErrShutdownGracefully
//   - The context is canceled (SIGINT/SIGTERM)
//   - EOF is reached on stdin (Ctrl+D)
//
// Errors from individual commands are printed to stderr but do not terminate
// the loop. Only ErrShutdownGracefully causes the loop to return that error.
func Loop(ctx context.Context, d Dispatcher, stdin io.Reader, stderr io.Writer) error {
	scanner := bufio.NewScanner(stdin)

	for {
		// Check for context cancellation before reading input.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Print prompt.
		fmt.Print("> ")

		// Read next line.
		if !scanner.Scan() {
			// EOF or error.
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("repl: read input: %w", err)
			}
			// EOF (Ctrl+D) - clean exit.
			fmt.Println() // Newline after prompt for clean exit.
			return nil
		}

		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines.
		if line == "" {
			continue
		}

		// Dispatch the input.
		err := d.DispatchInput(ctx, line)
		if err != nil {
			if errors.Is(err, registry.ErrShutdownGracefully) {
				return err
			}
			// Print error but continue loop.
			fmt.Fprintf(stderr, "Error: %v\n", err)
		}
	}
}

// Package repl provides the Read-Eval-Print Loop functionality for CodeMint.
package repl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

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

// MuxDispatcher extends Dispatcher with source-aware input dispatch.
// Implementations record the Source and UserID from InboundMessage
// into coordination task metadata for audit trails.
type MuxDispatcher interface {
	// DispatchInbound routes an inbound message with full metadata.
	DispatchInbound(ctx context.Context, msg InboundMessage) error
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

// LoopWithMux runs the REPL using an InputMultiplexer for multi-source input.
// It automatically registers the TUI source and reads from stdin, while also
// accepting inbound messages from other sources (CUI, Telegram, etc.).
//
// The dispatcher must implement MuxDispatcher to receive full InboundMessage
// metadata including Source and UserID for audit logging.
//
// Exits when:
//   - A command returns registry.ErrShutdownGracefully
//   - The context is canceled (SIGINT/SIGTERM)
//   - EOF is reached on stdin (Ctrl+D)
func LoopWithMux(ctx context.Context, d MuxDispatcher, mux *InputMultiplexer, stdin io.Reader, stderr io.Writer) error {
	// Register TUI source for stdin.
	tuiInbox := mux.RegisterSource("tui", 16)

	// Start stdin reader goroutine.
	stdinDone := make(chan error, 1)
	go func() {
		defer close(tuiInbox)
		scanner := bufio.NewScanner(stdin)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			tuiInbox <- InboundMessage{
				Source: "tui",
				Text:   line,
				At:     time.Now(),
			}
		}
		if err := scanner.Err(); err != nil {
			stdinDone <- fmt.Errorf("repl: read input: %w", err)
		}
		stdinDone <- nil
	}()

	// Main dispatch loop: read from multiplexer output.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case err := <-stdinDone:
			// stdin closed (EOF/Ctrl+D) - clean exit.
			if err != nil {
				return err
			}
			fmt.Println() // Newline after prompt for clean exit.
			return nil

		case msg, ok := <-mux.Recv():
			if !ok {
				// Multiplexer closed.
				return nil
			}

			// Print prompt for non-TUI sources so output looks consistent.
			if msg.Source != "tui" {
				fmt.Printf("[%s] > %s\n", msg.Source, msg.Text)
			} else {
				fmt.Print("> ")
			}

			err := d.DispatchInbound(ctx, msg)
			if err != nil {
				if errors.Is(err, registry.ErrShutdownGracefully) {
					return err
				}
				// Print error but continue loop.
				fmt.Fprintf(stderr, "Error: %v\n", err)
			}
		}
	}
}

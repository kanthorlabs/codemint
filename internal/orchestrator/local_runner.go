package orchestrator

import (
	"bytes"
	"context"
	"os/exec"
	"time"

	"github.com/google/shlex"
)

// Default configuration values for LocalRunner.
const (
	// DefaultRunTimeout is the default timeout for command execution.
	DefaultRunTimeout = 60 * time.Second
	// DefaultMaxOutputSize is the maximum buffer size for stdout/stderr (64 KiB).
	DefaultMaxOutputSize = 64 * 1024
	// TruncationMarker is appended to output that exceeds the buffer limit.
	TruncationMarker = "\n[truncated]"
)

// RunResult contains the outcome of a command execution.
type RunResult struct {
	// ExitCode is the process exit code (0 for success).
	ExitCode int
	// Stdout contains the captured standard output (truncated if too large).
	Stdout string
	// Stderr contains the captured standard error (truncated if too large).
	Stderr string
	// Duration is the wall-clock time the command took to execute.
	Duration time.Duration
	// Killed indicates whether the process was terminated due to timeout.
	Killed bool
}

// LocalRunner executes shell commands locally with bounded output capture.
type LocalRunner struct {
	// Timeout is the maximum duration for command execution.
	// Defaults to DefaultRunTimeout (60s) if not set.
	Timeout time.Duration
	// MaxOutputSize is the maximum buffer size for stdout/stderr.
	// Defaults to DefaultMaxOutputSize (64 KiB) if not set.
	MaxOutputSize int
}

// NewLocalRunner creates a new LocalRunner with default configuration.
func NewLocalRunner() *LocalRunner {
	return &LocalRunner{
		Timeout:       DefaultRunTimeout,
		MaxOutputSize: DefaultMaxOutputSize,
	}
}

// Run executes a command line in the specified working directory.
// It tokenizes the command using shlex, runs it via exec.CommandContext,
// and captures stdout/stderr to bounded buffers.
//
// The timeout parameter overrides the runner's default timeout if non-zero.
// Environment variables are NOT expanded; the command is passed directly.
func (r *LocalRunner) Run(ctx context.Context, cmdline, cwd string, timeout time.Duration) (RunResult, error) {
	var result RunResult
	start := time.Now()

	// Tokenize the command line
	args, err := shlex.Split(cmdline)
	if err != nil {
		return result, err
	}
	if len(args) == 0 {
		return result, exec.ErrNotFound
	}

	// Apply timeout (use provided timeout or fallback to runner's default)
	effectiveTimeout := timeout
	if effectiveTimeout == 0 {
		effectiveTimeout = r.Timeout
	}
	if effectiveTimeout == 0 {
		effectiveTimeout = DefaultRunTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, effectiveTimeout)
	defer cancel()

	// Build the command
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = cwd

	// Set up bounded output buffers
	maxSize := r.MaxOutputSize
	if maxSize == 0 {
		maxSize = DefaultMaxOutputSize
	}
	stdout := newBoundedBuffer(maxSize)
	stderr := newBoundedBuffer(maxSize)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	// Execute the command
	err = cmd.Run()
	result.Duration = time.Since(start)
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()

	// Check if killed due to context deadline
	if ctx.Err() == context.DeadlineExceeded {
		result.Killed = true
	}

	// Extract exit code
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else if result.Killed {
			// Killed processes typically have exit code -1
			result.ExitCode = -1
		} else {
			// Other errors (e.g., command not found)
			return result, err
		}
	}

	return result, nil
}

// boundedBuffer is a bytes.Buffer that limits total size and truncates with a marker.
type boundedBuffer struct {
	buf      bytes.Buffer
	maxSize  int
	overflow bool
}

// newBoundedBuffer creates a new bounded buffer with the specified maximum size.
func newBoundedBuffer(maxSize int) *boundedBuffer {
	return &boundedBuffer{maxSize: maxSize}
}

// Write implements io.Writer. Data is written until maxSize is reached,
// then additional writes are silently discarded.
func (b *boundedBuffer) Write(p []byte) (n int, err error) {
	if b.overflow {
		// Already at limit, discard but report success
		return len(p), nil
	}

	remaining := b.maxSize - b.buf.Len()
	if remaining <= 0 {
		b.overflow = true
		return len(p), nil
	}

	if len(p) > remaining {
		// Partial write
		b.buf.Write(p[:remaining])
		b.overflow = true
		return len(p), nil
	}

	return b.buf.Write(p)
}

// String returns the buffer contents, with a truncation marker if overflow occurred.
func (b *boundedBuffer) String() string {
	s := b.buf.String()
	if b.overflow {
		return s + TruncationMarker
	}
	return s
}

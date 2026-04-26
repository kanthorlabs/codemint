package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalRunner_Captures(t *testing.T) {
	runner := NewLocalRunner()
	ctx := context.Background()

	result, err := runner.Run(ctx, "echo hello", os.TempDir(), 0)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d; want 0", result.ExitCode)
	}

	expected := "hello\n"
	if result.Stdout != expected {
		t.Errorf("Stdout = %q; want %q", result.Stdout, expected)
	}

	if result.Stderr != "" {
		t.Errorf("Stderr = %q; want empty", result.Stderr)
	}

	if result.Killed {
		t.Error("Killed = true; want false")
	}

	if result.Duration == 0 {
		t.Error("Duration = 0; want > 0")
	}
}

func TestLocalRunner_CapturesStderr(t *testing.T) {
	runner := NewLocalRunner()
	ctx := context.Background()

	// Use sh -c to redirect to stderr
	result, err := runner.Run(ctx, `sh -c "echo error >&2"`, os.TempDir(), 0)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d; want 0", result.ExitCode)
	}

	expected := "error\n"
	if result.Stderr != expected {
		t.Errorf("Stderr = %q; want %q", result.Stderr, expected)
	}
}

func TestLocalRunner_NonZeroExitCode(t *testing.T) {
	runner := NewLocalRunner()
	ctx := context.Background()

	result, err := runner.Run(ctx, `sh -c "exit 42"`, os.TempDir(), 0)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode != 42 {
		t.Errorf("ExitCode = %d; want 42", result.ExitCode)
	}

	if result.Killed {
		t.Error("Killed = true; want false")
	}
}

func TestLocalRunner_Timeout(t *testing.T) {
	runner := NewLocalRunner()
	ctx := context.Background()

	// Run sleep with a very short timeout
	result, err := runner.Run(ctx, "sleep 5", os.TempDir(), 200*time.Millisecond)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !result.Killed {
		t.Error("Killed = false; want true")
	}

	// Duration should be around 200ms, not 5s
	if result.Duration > time.Second {
		t.Errorf("Duration = %v; want < 1s (timeout should have triggered)", result.Duration)
	}
}

func TestLocalRunner_WorkingDirectory(t *testing.T) {
	runner := NewLocalRunner()
	ctx := context.Background()

	// Create a temp directory
	tmpDir := t.TempDir()

	result, err := runner.Run(ctx, "pwd", tmpDir, 0)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d; want 0", result.ExitCode)
	}

	// The output should contain the temp directory path
	// Note: On macOS, /var is a symlink to /private/var, so we resolve both
	expectedDir, _ := filepath.EvalSymlinks(tmpDir)
	actualDir := strings.TrimSpace(result.Stdout)
	actualDir, _ = filepath.EvalSymlinks(actualDir)

	if actualDir != expectedDir {
		t.Errorf("pwd output = %q; want %q", actualDir, expectedDir)
	}
}

func TestLocalRunner_CommandNotFound(t *testing.T) {
	runner := NewLocalRunner()
	ctx := context.Background()

	_, err := runner.Run(ctx, "nonexistent_command_xyz123", os.TempDir(), 0)
	if err == nil {
		t.Fatal("Run() error = nil; want error for nonexistent command")
	}
}

func TestLocalRunner_EmptyCommand(t *testing.T) {
	runner := NewLocalRunner()
	ctx := context.Background()

	_, err := runner.Run(ctx, "", os.TempDir(), 0)
	if err == nil {
		t.Fatal("Run() error = nil; want error for empty command")
	}
}

func TestLocalRunner_Truncation(t *testing.T) {
	// Create a runner with a very small buffer
	runner := &LocalRunner{
		Timeout:       DefaultRunTimeout,
		MaxOutputSize: 10,
	}
	ctx := context.Background()

	// Generate output larger than the buffer
	result, err := runner.Run(ctx, "echo 12345678901234567890", os.TempDir(), 0)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should be truncated with marker
	if !strings.HasSuffix(result.Stdout, TruncationMarker) {
		t.Errorf("Stdout = %q; want truncation marker suffix", result.Stdout)
	}

	// Length should be MaxOutputSize + truncation marker
	expectedLen := 10 + len(TruncationMarker)
	if len(result.Stdout) != expectedLen {
		t.Errorf("len(Stdout) = %d; want %d", len(result.Stdout), expectedLen)
	}
}

func TestLocalRunner_QuotedArguments(t *testing.T) {
	runner := NewLocalRunner()
	ctx := context.Background()

	// Test that shlex correctly parses quoted arguments
	result, err := runner.Run(ctx, `echo "hello world"`, os.TempDir(), 0)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	expected := "hello world\n"
	if result.Stdout != expected {
		t.Errorf("Stdout = %q; want %q", result.Stdout, expected)
	}
}

func TestLocalRunner_ContextCancellation(t *testing.T) {
	runner := NewLocalRunner()
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	result, err := runner.Run(ctx, "sleep 5", os.TempDir(), 0)
	// Either error or killed
	if err == nil && !result.Killed {
		t.Error("Expected either error or killed result for cancelled context")
	}
}

func TestBoundedBuffer_UnderLimit(t *testing.T) {
	buf := newBoundedBuffer(100)

	n, err := buf.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != 5 {
		t.Errorf("Write() = %d; want 5", n)
	}

	if buf.String() != "hello" {
		t.Errorf("String() = %q; want %q", buf.String(), "hello")
	}

	if buf.overflow {
		t.Error("overflow = true; want false")
	}
}

func TestBoundedBuffer_ExactLimit(t *testing.T) {
	buf := newBoundedBuffer(5)

	n, err := buf.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != 5 {
		t.Errorf("Write() = %d; want 5", n)
	}

	// Should NOT have truncation marker at exact limit
	if buf.String() != "hello" {
		t.Errorf("String() = %q; want %q", buf.String(), "hello")
	}
}

func TestBoundedBuffer_OverLimit(t *testing.T) {
	buf := newBoundedBuffer(5)

	// First write fills the buffer
	buf.Write([]byte("hello"))

	// Second write should be discarded
	n, err := buf.Write([]byte("world"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != 5 {
		t.Errorf("Write() = %d; want 5 (reports full length even if discarded)", n)
	}

	// Should have truncation marker
	expected := "hello" + TruncationMarker
	if buf.String() != expected {
		t.Errorf("String() = %q; want %q", buf.String(), expected)
	}
}

func TestBoundedBuffer_PartialWrite(t *testing.T) {
	buf := newBoundedBuffer(7)

	// First write
	buf.Write([]byte("hello"))

	// Second write partially fits
	n, err := buf.Write([]byte("world"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != 5 {
		t.Errorf("Write() = %d; want 5", n)
	}

	// Should have "hello" + "wo" + truncation marker
	expected := "hellowo" + TruncationMarker
	if buf.String() != expected {
		t.Errorf("String() = %q; want %q", buf.String(), expected)
	}
}

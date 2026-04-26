package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repl"
)

// noopUI is a UIMediator that discards all output.
type noopUI struct{}

func (noopUI) RenderMessage(_ string)          {}
func (noopUI) ClearScreen()                    {}
func (noopUI) NotifyAll(_ registry.UIEvent)    {}
func (noopUI) PromptDecision(_ context.Context, _ registry.PromptRequest) registry.PromptResponse {
	return registry.PromptResponse{}
}

// captureUI is a UIMediator that records rendered messages and clear calls.
type captureUI struct {
	messages []string
	cleared  int
}

func (c *captureUI) RenderMessage(msg string)       { c.messages = append(c.messages, msg) }
func (c *captureUI) ClearScreen()                   { c.cleared++ }
func (c *captureUI) NotifyAll(_ registry.UIEvent)   {}
func (c *captureUI) PromptDecision(_ context.Context, _ registry.PromptRequest) registry.PromptResponse {
	return registry.PromptResponse{}
}

// newTestRegistry returns a registry pre-loaded with a "mock" command whose
// handler records the args it was called with.
func newTestRegistry(captureArgs *[]string, captureRaw *string) *registry.CommandRegistry {
	r := registry.NewCommandRegistry()
	_ = r.Register(registry.Command{
		Name:        "mock",
		Description: "Test command.",
		Usage:       "/mock [args...]",
		Handler: func(_ context.Context, _ registry.ActiveSessionInfo, args []string, rawArgs string) (registry.CommandResult, error) {
			*captureArgs = args
			*captureRaw = rawArgs
			return registry.CommandResult{}, nil
		},
	})
	return r
}

// cliSession returns an ActiveSession in CLI mode for test convenience.
func cliSession() *ActiveSession {
	return &ActiveSession{ClientMode: registry.ClientModeCLI, IsGlobal: true}
}

// daemonSession returns an ActiveSession in Daemon mode for test convenience.
func daemonSession() *ActiveSession {
	return &ActiveSession{ClientMode: registry.ClientModeDaemon, IsGlobal: true}
}

// --- Test A: Strict Parse ---

// TestDispatch_SlashCommand_ArgsPassedToHandler asserts that shell-split args
// and the raw argument string are forwarded intact to the command handler.
func TestDispatch_SlashCommand_ArgsPassedToHandler(t *testing.T) {
	var gotArgs []string
	var gotRaw string

	r := newTestRegistry(&gotArgs, &gotRaw)
	d := NewDispatcher(r, noopUI{}, nil)

	input := `/mock -t "buy milk" -p high`
	if err := d.Dispatch(context.Background(), cliSession(), input); err != nil {
		t.Fatalf("Dispatch returned unexpected error: %v", err)
	}

	wantArgs := []string{"-t", "buy milk", "-p", "high"}
	if len(gotArgs) != len(wantArgs) {
		t.Fatalf("args: got %v, want %v", gotArgs, wantArgs)
	}
	for i, a := range wantArgs {
		if gotArgs[i] != a {
			t.Errorf("args[%d]: got %q, want %q", i, gotArgs[i], a)
		}
	}

	wantRaw := `-t "buy milk" -p high`
	if gotRaw != wantRaw {
		t.Errorf("rawArgs: got %q, want %q", gotRaw, wantRaw)
	}
}

// TestDispatch_UnknownSlashCommand returns an error wrapping ErrCommandNotFound.
func TestDispatch_UnknownSlashCommand(t *testing.T) {
	r := registry.NewCommandRegistry()
	d := NewDispatcher(r, noopUI{}, nil)

	err := d.Dispatch(context.Background(), cliSession(), "/nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown command, got nil")
	}
	if !errors.Is(err, registry.ErrCommandNotFound) {
		t.Errorf("expected ErrCommandNotFound in chain, got: %v", err)
	}
}

// --- Declarative capability enforcement ---

// TestDispatch_CLIOnlyCommand_BlockedInDaemonMode asserts that a CLI-only
// command is rejected in daemon mode without calling the handler, and that a
// descriptive message is rendered via the UIMediator.
func TestDispatch_CLIOnlyCommand_BlockedInDaemonMode(t *testing.T) {
	r := registry.NewCommandRegistry()
	handlerCalled := false
	_ = r.Register(registry.Command{
		Name:           "exit",
		SupportedModes: []registry.ClientMode{registry.ClientModeCLI},
		Handler: func(_ context.Context, _ registry.ActiveSessionInfo, _ []string, _ string) (registry.CommandResult, error) {
			handlerCalled = true
			return registry.CommandResult{Action: registry.ActionExit}, nil
		},
	})

	ui := &captureUI{}
	d := NewDispatcher(r, ui, nil)

	err := d.Dispatch(context.Background(), daemonSession(), "/exit")
	if err != nil {
		t.Fatalf("expected nil error for graceful block, got: %v", err)
	}
	if handlerCalled {
		t.Error("handler must NOT be called when mode constraint is violated")
	}
	if len(ui.messages) == 0 {
		t.Error("expected a UI message explaining the restriction, got none")
	}
	if !strings.Contains(ui.messages[0], "exit") {
		t.Errorf("restriction message should mention command name, got: %q", ui.messages[0])
	}
}

// TestDispatch_CLIOnlyCommand_AllowedInCLIMode asserts that the same command
// executes normally when the client is in CLI mode.
func TestDispatch_CLIOnlyCommand_AllowedInCLIMode(t *testing.T) {
	r := registry.NewCommandRegistry()
	handlerCalled := false
	_ = r.Register(registry.Command{
		Name:           "exit",
		SupportedModes: []registry.ClientMode{registry.ClientModeCLI},
		Handler: func(_ context.Context, _ registry.ActiveSessionInfo, _ []string, _ string) (registry.CommandResult, error) {
			handlerCalled = true
			return registry.CommandResult{Action: registry.ActionExit, Message: "Goodbye."}, nil
		},
	})

	ui := &captureUI{}
	d := NewDispatcher(r, ui, nil)

	err := d.Dispatch(context.Background(), cliSession(), "/exit")
	if !errors.Is(err, ErrShutdownGracefully) {
		t.Errorf("expected ErrShutdownGracefully, got: %v", err)
	}
	if !handlerCalled {
		t.Error("handler should have been called in CLI mode")
	}
}

// TestDispatch_ActionExit_ReturnsErrShutdownGracefully asserts that ActionExit
// causes Dispatch to return ErrShutdownGracefully and renders the exit message.
func TestDispatch_ActionExit_ReturnsErrShutdownGracefully(t *testing.T) {
	r := registry.NewCommandRegistry()
	_ = r.Register(registry.Command{
		Name: "exit",
		Handler: func(_ context.Context, _ registry.ActiveSessionInfo, _ []string, _ string) (registry.CommandResult, error) {
			return registry.CommandResult{Action: registry.ActionExit, Message: "Goodbye."}, nil
		},
	})

	ui := &captureUI{}
	d := NewDispatcher(r, ui, nil)

	err := d.Dispatch(context.Background(), cliSession(), "/exit")
	if !errors.Is(err, ErrShutdownGracefully) {
		t.Errorf("expected ErrShutdownGracefully, got: %v", err)
	}
	if len(ui.messages) == 0 || ui.messages[0] != "Goodbye." {
		t.Errorf("exit message not rendered: %v", ui.messages)
	}
}

// TestDispatch_ActionClear_CallsUIClearScreen asserts that ActionClear triggers
// UIMediator.ClearScreen without returning an error.
func TestDispatch_ActionClear_CallsUIClearScreen(t *testing.T) {
	r := registry.NewCommandRegistry()
	_ = r.Register(registry.Command{
		Name: "clear",
		Handler: func(_ context.Context, _ registry.ActiveSessionInfo, _ []string, _ string) (registry.CommandResult, error) {
			return registry.CommandResult{Action: registry.ActionClear}, nil
		},
	})

	ui := &captureUI{}
	d := NewDispatcher(r, ui, nil)

	if err := d.Dispatch(context.Background(), cliSession(), "/clear"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ui.cleared != 1 {
		t.Errorf("ClearScreen called %d times, want 1", ui.cleared)
	}
}

// --- Test B: System Assistant Fallback ---

func TestDispatch_NaturalLanguage_GlobalSession_CallsSystemAssistant(t *testing.T) {
	var capturedInput string
	assistant := func(_ context.Context, input string) error {
		capturedInput = input
		return nil
	}

	r := registry.NewCommandRegistry()
	d := NewDispatcher(r, noopUI{}, assistant)

	input := "what is the status of the project?"
	if err := d.Dispatch(context.Background(), cliSession(), input); err != nil {
		t.Fatalf("Dispatch returned unexpected error: %v", err)
	}
	if capturedInput != input {
		t.Errorf("assistant received %q, want %q", capturedInput, input)
	}
}

func TestDispatch_NaturalLanguage_ProjectSession_ReturnsBrainstormerError(t *testing.T) {
	r := registry.NewCommandRegistry()
	d := NewDispatcher(r, noopUI{}, nil)
	active := &ActiveSession{ClientMode: registry.ClientModeCLI, IsGlobal: false}

	err := d.Dispatch(context.Background(), active, "add a login endpoint")
	if err == nil {
		t.Fatal("expected ErrNoBrainstormer, got nil")
	}
	if !errors.Is(err, ErrNoBrainstormer) {
		t.Errorf("expected ErrNoBrainstormer in chain, got: %v", err)
	}
}

// --- Test C: Help Generation ---

// TestHelp_CLIMode_ShowsAllCoreCommands asserts that /help in CLI mode shows
// all three core commands (help, exit, clear).
func TestHelp_CLIMode_ShowsAllCoreCommands(t *testing.T) {
	r := registry.NewCommandRegistry()
	if err := repl.RegisterCoreCommands(r); err != nil {
		t.Fatalf("RegisterCoreCommands: %v", err)
	}

	help := r.HelpTextForMode(registry.ClientModeCLI)

	for _, name := range []string{"help", "exit", "clear"} {
		if !strings.Contains(help, "/"+name) {
			t.Errorf("CLI help missing /%s\nfull output:\n%s", name, help)
		}
	}
}

// TestHelp_DaemonMode_HidesCLIOnlyCommands asserts that /help in daemon mode
// omits CLI-only commands (exit, clear) and retains shared ones (help).
func TestHelp_DaemonMode_HidesCLIOnlyCommands(t *testing.T) {
	r := registry.NewCommandRegistry()
	if err := repl.RegisterCoreCommands(r); err != nil {
		t.Fatalf("RegisterCoreCommands: %v", err)
	}

	help := r.HelpTextForMode(registry.ClientModeDaemon)

	if !strings.Contains(help, "/help") {
		t.Errorf("daemon help should contain /help\nfull output:\n%s", help)
	}
	for _, name := range []string{"exit", "clear"} {
		if strings.Contains(help, "/"+name) {
			t.Errorf("daemon help must NOT contain /%s\nfull output:\n%s", name, help)
		}
	}
}

// TestHelp_DomainCommandRegisteredAfterBoot_AppearsInHelp asserts that commands
// registered after RegisterCoreCommands are included in the help output.
func TestHelp_DomainCommandRegisteredAfterBoot_AppearsInHelp(t *testing.T) {
	r := registry.NewCommandRegistry()
	if err := repl.RegisterCoreCommands(r); err != nil {
		t.Fatalf("RegisterCoreCommands: %v", err)
	}
	if err := r.Register(registry.Command{
		Name:        "status",
		Description: "Show project status.",
		Usage:       "/status",
		Handler: func(_ context.Context, _ registry.ActiveSessionInfo, _ []string, _ string) (registry.CommandResult, error) {
			return registry.CommandResult{}, nil
		},
	}); err != nil {
		t.Fatalf("register /status: %v", err)
	}

	help := r.HelpText()
	if !strings.Contains(help, "/status") {
		t.Errorf("help text missing /status\nfull output:\n%s", help)
	}
}

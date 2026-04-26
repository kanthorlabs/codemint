// Package registry maintains the global slash-command registry used by the
// REPL dispatcher. Domain packages register their commands at boot time via
// Register; the dispatcher looks them up by name at runtime.
package registry

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
)

// ErrCommandNotFound is returned by Lookup when the requested command name is
// not present in the registry.
var ErrCommandNotFound = errors.New("registry: command not found")

// ErrDuplicateCommand is returned by Register when a command with the same
// name has already been registered.
var ErrDuplicateCommand = errors.New("registry: duplicate command name")

// ErrShutdownGracefully is returned when a command requests a clean process
// exit (ActionExit). Callers should treat this as a signal to flush state
// and terminate rather than as an unexpected error. This error is defined
// here to avoid import cycles between orchestrator and repl packages.
var ErrShutdownGracefully = errors.New("registry: shutdown requested")

// ClientMode describes the runtime environment in which CodeMint is operating.
// Defining it here (rather than in the orchestrator package) avoids an import
// cycle, since the orchestrator imports the registry.
type ClientMode string

const (
	// ClientModeCLI is a standard interactive terminal (TUI).
	ClientModeCLI ClientMode = "cli"
	// ClientModeDaemon is a background server attached to a Chat UI (CUI).
	// Commands that perform terminal-only actions (e.g. /exit, /clear) must
	// not be executed in this mode.
	ClientModeDaemon ClientMode = "daemon"
)

// SystemAction defines a lifecycle intent that a command can request from the
// Orchestrator or UIMediator. Keeping these as return values — rather than
// calling os.Exit or exec.Command directly inside a handler — makes the same
// handler safe in both TUI and CUI environments.
type SystemAction int

const (
	// ActionNone means the handler completed normally with no lifecycle side-effect.
	ActionNone SystemAction = iota
	// ActionExit requests a clean shutdown: flush DB, close connections, exit.
	ActionExit
	// ActionClear requests the UIMediator to clear the visible screen / chat viewport.
	ActionClear
)

// CommandResult encapsulates the structured output of a command handler.
// Message carries Markdown text for the UI to render; Action carries any
// system-level lifecycle intent.
type CommandResult struct {
	// Message is the text to display to the user. Markdown is supported.
	Message string
	// Action is the system lifecycle action requested by the handler.
	Action SystemAction
}

// ActiveSessionInfo is a minimal read-only view of the current session that
// handlers may inspect (e.g. to scope a query to the active project). It is
// an interface so that the registry package remains decoupled from the
// concrete orchestrator.ActiveSession struct.
type ActiveSessionInfo interface {
	GetClientMode() ClientMode
	GetIsGlobal() bool
}

// MutableSessionInfo extends ActiveSessionInfo with methods to modify session state.
// Used by commands that need to switch sessions or update session ownership.
type MutableSessionInfo interface {
	ActiveSessionInfo
	// GetSessionID returns the current session ID, or empty if global mode.
	GetSessionID() string
	// GetClientID returns the unique identifier for this client instance.
	GetClientID() string
	// SetSession updates the active session and project.
	SetSession(session any, project any, yoloEnabled bool)
	// SetSuspended marks the session as suspended (another client took over).
	SetSuspended(suspended bool)
	// SetClientMode changes the runtime mode (cli or daemon).
	SetClientMode(mode ClientMode)
}

// Handler is the unified signature for all slash-command implementations.
// active provides read-only session context; args are shell-tokenised
// arguments; rawArgs is the untouched argument string for System Assistant
// extraction. Returns a CommandResult for UI/system intent and an error ONLY
// for actual execution failures.
type Handler func(ctx context.Context, active ActiveSessionInfo, args []string, rawArgs string) (CommandResult, error)

// Command describes a single slash command.
type Command struct {
	// Name is the bare command word without the leading slash (e.g. "help").
	Name string
	// Description is a one-line summary shown in /help output.
	Description string
	// Usage is the full usage synopsis (e.g. "/ticket -t <title> -p <priority>").
	Usage string
	// SupportedModes declares which ClientModes may execute this command.
	// An empty slice means the command is available in all modes.
	SupportedModes []ClientMode
	// Handler is called by the dispatcher after mode constraints are satisfied.
	Handler Handler
}

// SupportsMode reports whether c is allowed to run in mode. A command with no
// declared SupportedModes is treated as universally available.
func (c Command) SupportsMode(mode ClientMode) bool {
	if len(c.SupportedModes) == 0 {
		return true
	}
	return slices.Contains(c.SupportedModes, mode)
}

// UIEventType categorizes the kind of event being broadcast to UI adapters.
type UIEventType string

const (
	// EventTaskStarted indicates a task has begun execution.
	EventTaskStarted UIEventType = "task_started"
	// EventTaskCompleted indicates a task finished successfully.
	EventTaskCompleted UIEventType = "task_completed"
	// EventTaskFailed indicates a task encountered an error.
	EventTaskFailed UIEventType = "task_failed"
	// EventAgentCrashed indicates an agent process terminated unexpectedly.
	EventAgentCrashed UIEventType = "agent_crashed"
	// EventSessionCreated indicates a new session was created.
	EventSessionCreated UIEventType = "session_created"
	// EventProgress indicates an incremental progress update.
	EventProgress UIEventType = "progress"
	// EventSessionTakeover indicates another client has taken over the session.
	// The payload contains the new owner's client ID.
	EventSessionTakeover UIEventType = "session_takeover"
	// EventSessionReclaimed indicates a suspended client has reclaimed the session.
	// The payload contains the reclaiming client's ID.
	EventSessionReclaimed UIEventType = "session_reclaimed"
	// EventACPStream indicates a classified event from the ACP agent stream.
	// The payload is an acp.Event struct carrying Kind, Raw, etc.
	EventACPStream UIEventType = "acp_stream"
	// EventACPAutoApproved indicates a command was auto-approved and executed
	// based on the project's permission whitelist. The payload contains command
	// details and execution result for audit purposes.
	EventACPAutoApproved UIEventType = "acp_auto_approved"
	// EventACPAwaitingApproval indicates a command requires human approval.
	// The payload contains the pending approval details.
	EventACPAwaitingApproval UIEventType = "acp_awaiting_approval"
	// EventACPApprovalResolved indicates a pending approval was resolved (approved/denied).
	EventACPApprovalResolved UIEventType = "acp_approval_resolved"
)

// UIEvent represents a fire-and-forget notification broadcast to all UI
// adapters. Unlike PromptRequest, events do not block waiting for a response.
type UIEvent struct {
	// Type categorizes the event (e.g., "task_started", "task_completed").
	Type UIEventType
	// TaskID is the task this event relates to (optional, may be empty).
	TaskID string
	// Message is a human-readable description for UI rendering.
	Message string
	// Payload carries optional structured data for rich UI rendering.
	// Adapters may type-assert to extract event-specific details.
	Payload any
}

// PromptKind categorizes the type of decision prompt.
type PromptKind string

const (
	// PromptKindGeneral is a general-purpose prompt (default).
	PromptKindGeneral PromptKind = "general"
	// PromptKindACPCommandApproval is used for ACP blocked/unknown command approval.
	PromptKindACPCommandApproval PromptKind = "acp_command_approval"
)

// PromptOption represents a selectable choice in a decision prompt.
type PromptOption struct {
	ID          string // Unique identifier for programmatic handling
	Label       string // Human-readable display text
	Description string // Optional detailed explanation
}

// PromptRequest contains the information needed to display a decision prompt
// to the user across multiple UI adapters.
type PromptRequest struct {
	// Kind categorizes the type of prompt (e.g., "acp_command_approval").
	Kind PromptKind
	// TaskID is the task this prompt relates to (optional, may be empty).
	TaskID string
	// Title is a short header for the prompt.
	Title string
	// Body is the main content/question being asked.
	Body string
	// Message is a human-readable description (legacy field, prefer Body).
	Message string
	// Options are the selectable choices (legacy field, prefer PromptOptions).
	Options []string // e.g., ["Accept", "Revert"]
	// PromptOptions are structured choices with ID, Label, and Description.
	PromptOptions []PromptOption
}

// PromptResponse carries the user's selected option from a UI adapter.
type PromptResponse struct {
	// SelectedOption is the label of the selected option (legacy).
	SelectedOption string
	// SelectedOptionID is the ID of the selected PromptOption.
	SelectedOptionID string
	// Error is set if the prompt failed or was cancelled.
	Error error
}

// UIMediator abstracts all user-facing output and system interactions so that
// command handlers and the Dispatcher remain UI-agnostic. A TUI implementation
// writes to stdout; a CUI implementation enqueues chat bubbles.
type UIMediator interface {
	// RenderMessage displays msg to the user. Markdown is supported.
	RenderMessage(msg string)
	// ClearScreen resets the visible output area.
	ClearScreen()
	// NotifyAll broadcasts a fire-and-forget event to all registered UI adapters.
	// Events are delivered asynchronously; the method returns immediately.
	NotifyAll(event UIEvent)
	// PromptDecision broadcasts a decision prompt to all registered UI adapters
	// concurrently. The first adapter to respond wins; other adapters receive
	// context cancellation to dismiss their pending prompts.
	PromptDecision(ctx context.Context, req PromptRequest) PromptResponse
}

// CommandRegistry is a concurrency-safe store of named Commands.
type CommandRegistry struct {
	mu       sync.RWMutex
	commands map[string]Command
}

// NewCommandRegistry returns an empty CommandRegistry ready for use.
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{commands: make(map[string]Command)}
}

// Register adds c to the registry. It returns ErrDuplicateCommand if a
// command with the same Name has already been registered.
func (r *CommandRegistry) Register(c Command) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.commands[c.Name]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateCommand, c.Name)
	}
	r.commands[c.Name] = c
	return nil
}

// Lookup returns the Command registered under name. It returns
// ErrCommandNotFound when no such command exists.
func (r *CommandRegistry) Lookup(name string) (Command, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.commands[name]
	if !ok {
		return Command{}, fmt.Errorf("%w: %q", ErrCommandNotFound, name)
	}
	return c, nil
}

// All returns every registered Command sorted alphabetically by Name.
func (r *CommandRegistry) All() []Command {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Command, 0, len(r.commands))
	for _, c := range r.commands {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// ForMode returns every Command that supports mode, sorted alphabetically.
func (r *CommandRegistry) ForMode(mode ClientMode) []Command {
	all := r.All()
	out := make([]Command, 0, len(all))
	for _, c := range all {
		if c.SupportsMode(mode) {
			out = append(out, c)
		}
	}
	return out
}

// helpBlock renders one command entry for help output.
func helpBlock(sb *strings.Builder, c Command) {
	fmt.Fprintf(sb, "  /%s\n", c.Name)
	fmt.Fprintf(sb, "    %s\n", c.Description)
	if c.Usage != "" {
		fmt.Fprintf(sb, "    Usage: %s\n", c.Usage)
	}
	sb.WriteByte('\n')
}

// HelpText renders a formatted help table for all registered commands.
func (r *CommandRegistry) HelpText() string {
	cmds := r.All()
	if len(cmds) == 0 {
		return "No commands registered.\n"
	}
	var sb strings.Builder
	sb.WriteString("Available commands:\n\n")
	for _, c := range cmds {
		helpBlock(&sb, c)
	}
	return sb.String()
}

// HelpTextForMode renders a formatted help table showing only the commands
// available in mode. Used by the dynamic /help handler.
func (r *CommandRegistry) HelpTextForMode(mode ClientMode) string {
	cmds := r.ForMode(mode)
	if len(cmds) == 0 {
		return "No commands available in this mode.\n"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Available commands (%s mode):\n\n", mode)
	for _, c := range cmds {
		helpBlock(&sb, c)
	}
	return sb.String()
}

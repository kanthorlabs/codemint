package repl

import (
	"context"
	"fmt"
	"strings"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repository"
)

// ACPSessionInfo provides access to ACP-related session state.
// This interface breaks the import cycle between repl and orchestrator.
type ACPSessionInfo interface {
	registry.MutableSessionInfo
	// Project returns the active project, or nil.
	GetProject() *domain.Project
	// Session returns the active session, or nil.
	GetSession() *domain.Session
	// ACPRegistry returns the ACP worker registry.
	ACPRegistry() *acp.Registry
	// GetACPSessionID returns the ACP session ID.
	GetACPSessionID() string
	// SetACPSessionID sets the ACP session ID.
	SetACPSessionID(id string)
}

// ACPCommandDeps holds the dependencies needed for ACP-related commands.
type ACPCommandDeps struct {
	ActiveSession ACPSessionInfo
	TaskRepo      repository.TaskRepository
	AgentRepo     repository.AgentRepository
	UIMediator    registry.UIMediator
}

// RegisterACPCommands registers ACP worker commands (/acp, /acp-status, /acp-stop).
func RegisterACPCommands(r *registry.CommandRegistry, deps *ACPCommandDeps) error {
	commands := []registry.Command{
		{
			Name:           "acp",
			Description:    "Send a prompt to the ACP agent (OpenCode).",
			Usage:          "/acp <prompt>",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon},
			Handler:        acpPromptHandler(deps),
		},
		{
			Name:           "acp-status",
			Description:    "Show ACP worker status (pid, cwd, capabilities).",
			Usage:          "/acp-status",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon},
			Handler:        acpStatusHandler(deps),
		},
		{
			Name:           "acp-stop",
			Description:    "Stop the ACP worker for the current session.",
			Usage:          "/acp-stop",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon},
			Handler:        acpStopHandler(deps),
		},
	}

	for _, c := range commands {
		if err := r.Register(c); err != nil {
			return fmt.Errorf("repl: register acp command %q: %w", c.Name, err)
		}
	}
	return nil
}

// acpPromptHandler handles the /acp <prompt> command.
// It sends a prompt to the ACP agent and streams the response.
func acpPromptHandler(deps *ACPCommandDeps) registry.Handler {
	return func(ctx context.Context, active registry.ActiveSessionInfo, args []string, rawArgs string) (registry.CommandResult, error) {
		// Require a prompt.
		prompt := strings.TrimSpace(rawArgs)
		if prompt == "" {
			return registry.CommandResult{
				Message: "Usage: /acp <prompt>",
				Action:  registry.ActionNone,
			}, nil
		}

		// Check if we have a session.
		session := deps.ActiveSession.GetSession()
		if session == nil {
			return registry.CommandResult{
				Message: "No active session. Use /project-open to start.",
				Action:  registry.ActionNone,
			}, nil
		}

		project := deps.ActiveSession.GetProject()

		// Get the ACP registry.
		acpReg := deps.ActiveSession.ACPRegistry()
		if acpReg == nil {
			return registry.CommandResult{
				Message: "ACP registry not available.",
				Action:  registry.ActionNone,
			}, nil
		}

		// Get or spawn the worker.
		worker, err := acpReg.GetOrSpawn(ctx, session, project)
		if err != nil {
			return registry.CommandResult{
				Message: fmt.Sprintf("Failed to start ACP worker: %v", err),
				Action:  registry.ActionNone,
			}, nil
		}

		// Create a new ACP session if we don't have one.
		acpSessionID := deps.ActiveSession.GetACPSessionID()
		if acpSessionID == "" {
			acpSessionID, err = createACPSession(ctx, worker)
			if err != nil {
				return registry.CommandResult{
					Message: fmt.Sprintf("Failed to create ACP session: %v", err),
					Action:  registry.ActionNone,
				}, nil
			}
			deps.ActiveSession.SetACPSessionID(acpSessionID)
		}

		// Send the prompt.
		promptParams := acp.SessionPromptParams{
			SessionID: acpSessionID,
			Prompt:    prompt,
		}
		promptReq, err := acp.NewRequest(acp.MethodSessionPrompt, promptParams)
		if err != nil {
			return registry.CommandResult{}, fmt.Errorf("create prompt request: %w", err)
		}

		if err := worker.Send(promptReq); err != nil {
			return registry.CommandResult{
				Message: fmt.Sprintf("Failed to send prompt: %v", err),
				Action:  registry.ActionNone,
			}, nil
		}

		// Record the interaction as a coordination task if we have repos.
		if deps.TaskRepo != nil && deps.AgentRepo != nil && project != nil {
			go recordACPInteraction(context.Background(), deps, prompt)
		}

		// Stream responses back to the UI.
		var responseBuilder strings.Builder
		responseBuilder.WriteString("ACP response:\n\n")

		for {
			select {
			case <-ctx.Done():
				return registry.CommandResult{
					Message: responseBuilder.String() + "\n(cancelled)",
					Action:  registry.ActionNone,
				}, nil

			case msg, ok := <-worker.Out():
				if !ok {
					// Worker exited.
					if responseBuilder.Len() == 0 {
						return registry.CommandResult{
							Message: "ACP worker exited unexpectedly.",
							Action:  registry.ActionNone,
						}, nil
					}
					return registry.CommandResult{
						Message: responseBuilder.String(),
						Action:  registry.ActionNone,
					}, nil
				}

				// Handle session/update notifications.
				if msg.Method == acp.MethodSessionUpdate {
					update, err := msg.ParseSessionUpdate()
					if err != nil {
						continue
					}

					// Process different update types.
					switch update.Update.Kind {
					case acp.UpdateKindAgentMessageChunk:
						// Extract content from the raw update.
						content := extractChunkContent(update.Update.Raw)
						if content != "" {
							responseBuilder.WriteString(content)
							// Stream to UI immediately.
							if deps.UIMediator != nil {
								deps.UIMediator.RenderMessage(content)
							}
						}

					case acp.UpdateKindAgentThoughtChunk:
						// Show thoughts in a different format.
						content := extractChunkContent(update.Update.Raw)
						if content != "" {
							responseBuilder.WriteString("[thought] ")
							responseBuilder.WriteString(content)
						}

					case acp.UpdateKindToolCall:
						// Show tool calls.
						responseBuilder.WriteString("\n[tool call] ")

					case acp.UpdateKindPlan:
						// Agent finished.
						return registry.CommandResult{
							Message: responseBuilder.String(),
							Action:  registry.ActionNone,
						}, nil
					}
				}

				// Handle responses (for prompt acknowledgment).
				if msg.IsResponse() && msg.GetID() == promptReq.GetID() {
					if msg.Error != nil {
						return registry.CommandResult{
							Message: fmt.Sprintf("Prompt failed: %v", msg.Error),
							Action:  registry.ActionNone,
						}, nil
					}
					// Prompt was accepted, continue waiting for updates.
				}
			}
		}
	}
}

// acpStatusHandler handles the /acp-status command.
func acpStatusHandler(deps *ACPCommandDeps) registry.Handler {
	return func(ctx context.Context, active registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		// Check if we have a session.
		session := deps.ActiveSession.GetSession()
		if session == nil {
			return registry.CommandResult{
				Message: "No active session.",
				Action:  registry.ActionNone,
			}, nil
		}

		// Get the ACP registry.
		acpReg := deps.ActiveSession.ACPRegistry()
		if acpReg == nil {
			return registry.CommandResult{
				Message: "ACP registry not available.",
				Action:  registry.ActionNone,
			}, nil
		}

		// Check if we have a worker.
		worker, ok := acpReg.Get(session.ID)
		if !ok {
			return registry.CommandResult{
				Message: "No ACP worker running for this session.",
				Action:  registry.ActionNone,
			}, nil
		}

		// Build status message.
		var sb strings.Builder
		sb.WriteString("ACP Worker Status:\n\n")

		caps := worker.Capabilities()
		fmt.Fprintf(&sb, "  PID:        %d\n", worker.Pid())
		fmt.Fprintf(&sb, "  CWD:        %s\n", worker.Cwd())
		fmt.Fprintf(&sb, "  Server:     %s v%s\n", caps.ServerInfo.Name, caps.ServerInfo.Version)
		fmt.Fprintf(&sb, "  Streaming:  %v\n", caps.Capabilities.Streaming)
		fmt.Fprintf(&sb, "  Tool Calls: %v\n", caps.Capabilities.ToolCalls)
		fmt.Fprintf(&sb, "  Planning:   %v\n", caps.Capabilities.Planning)
		fmt.Fprintf(&sb, "  ACP Session: %s\n", deps.ActiveSession.GetACPSessionID())

		return registry.CommandResult{
			Message: sb.String(),
			Action:  registry.ActionNone,
		}, nil
	}
}

// acpStopHandler handles the /acp-stop command.
func acpStopHandler(deps *ACPCommandDeps) registry.Handler {
	return func(ctx context.Context, active registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		// Check if we have a session.
		session := deps.ActiveSession.GetSession()
		if session == nil {
			return registry.CommandResult{
				Message: "No active session.",
				Action:  registry.ActionNone,
			}, nil
		}

		// Get the ACP registry.
		acpReg := deps.ActiveSession.ACPRegistry()
		if acpReg == nil {
			return registry.CommandResult{
				Message: "ACP registry not available.",
				Action:  registry.ActionNone,
			}, nil
		}

		// Stop the worker.
		if err := acpReg.Stop(ctx, session.ID); err != nil {
			return registry.CommandResult{
				Message: fmt.Sprintf("Failed to stop ACP worker: %v", err),
				Action:  registry.ActionNone,
			}, nil
		}

		// Clear ACP session ID.
		deps.ActiveSession.SetACPSessionID("")

		return registry.CommandResult{
			Message: "ACP worker stopped.",
			Action:  registry.ActionNone,
		}, nil
	}
}

// createACPSession creates a new ACP session by sending session/new.
func createACPSession(ctx context.Context, worker *acp.Worker) (string, error) {
	req, err := acp.NewRequest(acp.MethodSessionNew, acp.SessionNewParams{})
	if err != nil {
		return "", fmt.Errorf("create session/new request: %w", err)
	}

	resp, err := worker.SendRequest(ctx, req)
	if err != nil {
		return "", fmt.Errorf("session/new: %w", err)
	}

	if resp.Error != nil {
		return "", resp.Error
	}

	var result acp.SessionNewResult
	if err := resp.ParseResult(&result); err != nil {
		return "", fmt.Errorf("parse session/new result: %w", err)
	}

	return result.SessionID, nil
}

// extractChunkContent extracts the content field from a chunk update.
func extractChunkContent(raw []byte) string {
	// Simple extraction - in production, use proper JSON parsing.
	// Looking for "content":"..." pattern.
	s := string(raw)
	start := strings.Index(s, `"content":"`)
	if start == -1 {
		return ""
	}
	start += len(`"content":"`)
	end := start
	for end < len(s) {
		if s[end] == '"' && (end == 0 || s[end-1] != '\\') {
			break
		}
		end++
	}
	if end > start {
		return s[start:end]
	}
	return ""
}

// recordACPInteraction records the ACP interaction as a coordination task.
func recordACPInteraction(ctx context.Context, deps *ACPCommandDeps, prompt string) {
	project := deps.ActiveSession.GetProject()
	session := deps.ActiveSession.GetSession()
	if project == nil || session == nil {
		return
	}

	// Get system agent.
	systemAgent, err := deps.AgentRepo.FindByName(ctx, "System")
	if err != nil || systemAgent == nil {
		return
	}

	// Create coordination task.
	task := domain.NewTask(
		project.ID,
		session.ID,
		"", // No workflow
		systemAgent.ID,
		domain.TaskTypeCoordination,
	)
	task.Input.String = fmt.Sprintf(`{"command":"/acp","prompt":"%s"}`, prompt)
	task.Input.Valid = true
	task.Status = domain.TaskStatusCompleted
	task.ClientID.String = deps.ActiveSession.GetClientID()
	task.ClientID.Valid = true

	_ = deps.TaskRepo.Create(ctx, task)
}

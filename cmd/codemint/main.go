// Package main is the entry point for the CodeMint CLI application.
// It wires together all components and starts the REPL loop.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/agent"
	appconfig "codemint.kanthorlabs.com/internal/config"
	"codemint.kanthorlabs.com/internal/db"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/orchestrator"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repl"
	"codemint.kanthorlabs.com/internal/repository/sqlite"
	"codemint.kanthorlabs.com/internal/ui"
	"codemint.kanthorlabs.com/internal/workflow"
	"codemint.kanthorlabs.com/internal/xdg"
)

// Build metadata injected via ldflags.
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

// config holds the parsed command-line flags.
type config struct {
	showVersion   bool
	showHelp      bool
	configPath    string
	dbPath        string
	mode          string
	withAssistant bool
}

// dispatcherWrapper adapts orchestrator.Dispatcher to repl.Dispatcher interface.
// It captures the active session so the REPL loop doesn't need to pass it.
type dispatcherWrapper struct {
	dispatcher *orchestrator.Dispatcher
	session    *orchestrator.ActiveSession
}

// DispatchInput implements repl.Dispatcher.
func (w *dispatcherWrapper) DispatchInput(ctx context.Context, input string) error {
	return w.dispatcher.Dispatch(ctx, w.session, input)
}

func main() {
	if err := run(); err != nil {
		if errors.Is(err, registry.ErrShutdownGracefully) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "codemint: %v\n", err)
		os.Exit(1)
	}
}

// run is the actual entry point, returning an error for testability.
// All os.Exit calls are confined to main().
func run() error {
	cfg := parseFlags()

	if cfg.showHelp {
		flag.Usage()
		return nil
	}

	if cfg.showVersion {
		fmt.Printf("codemint %s (commit: %s, built: %s)\n", version, commit, buildDate)
		return nil
	}

	// Validate mode flag.
	clientMode, err := parseClientMode(cfg.mode)
	if err != nil {
		return err
	}

	// Step 1: Ensure XDG directories exist before any other initialization.
	if err := xdg.EnsureDirs(); err != nil {
		return fmt.Errorf("initialize directories: %w", err)
	}

	// Resolve database path.
	dbPath := cfg.dbPath
	if dbPath == "" {
		dbPath = xdg.DatabasePath()
	}

	// Step 2: Create a cancellable context for graceful shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Step 3: Open database with proper pragmas (busy_timeout, WAL, foreign_keys).
	dbConn, err := db.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer dbConn.Close()

	// Step 4: Run migrations.
	if err := db.RunMigrations(dbConn); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	// Step 5: Create repositories with the DB connection.
	agentRepo := sqlite.NewAgentRepo(dbConn)
	taskRepo := sqlite.NewTaskRepo(dbConn)
	sessionRepo := sqlite.NewSessionRepo(dbConn)
	projectRepo := sqlite.NewProjectRepo(dbConn)
	workflowRepo := sqlite.NewWorkflowRepo(dbConn)

	// Step 6: Seed system agents.
	if err := agentRepo.EnsureSystemAgents(ctx); err != nil {
		return fmt.Errorf("seed system agents: %w", err)
	}

	// Step 6b: Ensure CodeMint sentinel project exists (Story 2.0).
	// This creates the project, workspace directory, permission row, and session if missing.
	permissionRepo := sqlite.NewProjectPermissionRepo(dbConn)
	if err := orchestrator.EnsureCodeMintProject(ctx, xdg.WorkspaceDir(), projectRepo, sessionRepo, permissionRepo); err != nil {
		return fmt.Errorf("ensure codemint project: %w", err)
	}

	// Step 7: Create command registry and register core commands.
	cmdRegistry := registry.NewCommandRegistry()
	if err := repl.RegisterCoreCommands(cmdRegistry); err != nil {
		return fmt.Errorf("register core commands: %w", err)
	}

	// Step 8: Create UI mediator.
	mediator := ui.NewUIMediator(os.Stdout)

	// Step 9: Load configuration and create workflow registry.
	appCfg, err := appconfig.Load(cfg.configPath)
	if err != nil {
		log.Printf("Warning: failed to load config from %s: %v", cfg.configPath, err)
		appCfg = &appconfig.Config{}
	}

	// Register builtin provider names with config validator for provider validation.
	appconfig.BuiltinProviderNames = agent.BuiltinProviderNames

	// Validate configuration (including provider references).
	if err := appconfig.Validate(appCfg); err != nil {
		log.Printf("Warning: config validation failed: %v", err)
	}

	var workflowReg *workflow.WorkflowRegistry
	if len(appCfg.Workflows) > 0 {
		workflowReg, err = workflow.LoadFromConfig(appCfg)
		if err != nil {
			return fmt.Errorf("load workflow registry: %w", err)
		}
		log.Printf("Loaded %d workflow(s) from config", workflowReg.Len())
	}

	// Step 9b: Create Workflow File Registry (Story 2.0.1).
	// Loads WORKFLOW.yaml files from embedded and external sources.
	workflowFileReg := workflow.NewFileRegistry()
	if err := workflowFileReg.LoadAll(); err != nil {
		log.Printf("Warning: failed to load workflow files: %v", err)
	} else if workflowFileReg.Len() > 0 {
		log.Printf("Loaded %d workflow file(s): %v", workflowFileReg.Len(), workflowFileReg.Names())
	}
	// Note: workflowFileReg will be passed to command registration in Story 2.0.4
	_ = workflowFileReg // Suppress unused warning until Story 2.0.4

	// Step 9c: Create Provider Registry (Story 3.22).
	// This merges builtin catalog with config overrides.
	providerRegistry, err := agent.NewProviderRegistry(appCfg)
	if err != nil {
		return fmt.Errorf("create provider registry: %w", err)
	}

	// Step 9d: Resolve System Assistant provider.
	// This handles CODEMINT_ACP_CMD env override for backward compatibility.
	systemProviderName := appCfg.Assistants.System.Provider
	systemProvider, providerErr := agent.ResolveSystemAssistantProvider(providerRegistry, systemProviderName)
	if providerErr != nil {
		log.Printf("Warning: failed to resolve system assistant provider %q: %v", systemProviderName, providerErr)
		systemProvider = nil
	} else {
		// Check if the binary exists
		if err := providerRegistry.MustExist(systemProvider.Name); err != nil {
			// If env override was used, the provider is "env-override" which isn't in registry
			// In that case, we still have a valid provider from ResolveSystemAssistantProvider
			if systemProvider.Name != "env-override" {
				log.Printf("Warning: %v — System Assistant will be disabled", err)
				systemProvider = nil
			}
		}
	}

	// Step 9e: Create ACP worker registry with provider-based config.
	// The registry is created lazily - workers are only spawned when needed.
	var acpRegistry *acp.Registry
	if systemProvider != nil {
		acpCfg := agent.WorkerConfigFromProvider(systemProvider, "")
		acpRegistry = acp.NewRegistry(acpCfg)
	} else {
		// Fallback to default config if no provider resolved
		acpRegistry = acp.NewRegistry(acp.DefaultConfig())
	}
	// Step 9f: Create ACP Runtime.
	// The Runtime wires together Pipeline, Interceptor, StatusMapper, Fanout, and BufferRegistry.
	bufferRegistry := acp.NewBufferRegistry(acp.DefaultBufferCapacity)

	acpRuntime, err := orchestrator.NewRuntime(ctx, orchestrator.RuntimeConfig{
		Registry:       acpRegistry,
		BufferRegistry: bufferRegistry,
		Mediator:       mediator,
		PermissionRepo: permissionRepo,
		TaskRepo:       taskRepo,
		SessionRepo:    sessionRepo,
		AgentRepo:      agentRepo,
	})
	if err != nil {
		log.Fatalf("Failed to create ACP runtime: %v", err)
	}

	// Step 9g: Create System Assistant if enabled (Story 3.19).
	var systemAssistant agent.SystemAssistant
	if cfg.withAssistant && systemProvider != nil {
		sa, saErr := agent.NewACPAssistant(agent.ACPAssistantConfig{
			Attacher: acpRuntime,
			Provider: systemProvider,
		})
		switch {
		case errors.Is(saErr, agent.ErrProviderBinaryMissing):
			log.Printf("Warning: %v — System Assistant disabled", saErr)
		case saErr != nil:
			return fmt.Errorf("system assistant: %w", saErr)
		default:
			systemAssistant = sa
			log.Printf("System Assistant ready (provider=%s)", sa.Provider().Name)
		}
	} else if cfg.withAssistant && systemProvider == nil {
		log.Printf("Warning: System Assistant disabled (no valid provider)")
	}

	// Step 10: Create dispatcher with system assistant.
	dispatcher := orchestrator.NewDispatcher(cmdRegistry, mediator, systemAssistant, workflowReg)

	// Set up interaction recorder for persisting user commands as Coordination tasks.
	interactionRecorder := orchestrator.NewInteractionRecorder(taskRepo, agentRepo)
	dispatcher.SetInteractionRecorder(interactionRecorder)

	// Step 11: Auto-load most recent active session.
	sessionLoader := orchestrator.NewSessionLoader(sessionRepo, projectRepo)
	loadResult, err := sessionLoader.LoadMostRecentSession(ctx, clientMode)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	// Create active session from load result.
	activeSession := sessionLoader.CreateActiveSession(loadResult, clientMode)

	// Step 11b: Create and register UI adapters based on client mode.
	// CLI mode: TUIAdapter for high-bandwidth terminal streaming.
	// Daemon mode: CUIAdapter for low-bandwidth pulse notifications.
	adapters, err := ui.BuildAdapters(clientMode, ui.AdapterConfig{
		Writer: os.Stdout,
		VerbosityGetter: func() ui.VerbosityLevel {
			return ui.VerbosityLevel(activeSession.GetVerbosity())
		},
	})
	if err != nil {
		return fmt.Errorf("build ui adapters: %w", err)
	}
	adapters.RegisterAll(mediator)

	// Ensure adapters are closed on exit.
	defer adapters.Close()

	// Set ACP references on active session.
	activeSession.SetACPRegistry(acpRegistry)
	activeSession.SetACPRuntime(acpRuntime)

	// Register callback to refresh permissions when the project changes (Task 3.12.3).
	activeSession.OnProjectSwitch(func(project *domain.Project) {
		if project == nil {
			return
		}
		// Refresh permissions for the new project.
		if err := acpRuntime.RefreshPermissions(context.Background(), project.ID); err != nil {
			log.Printf("Warning: failed to refresh permissions for project %s: %v", project.ID, err)
		}
	})

	// Ensure ACP workers and consumers are stopped on exit (graceful shutdown, SIGINT/SIGTERM, or panic).
	// Use a fresh context (not the canceled signal context) so children can be reaped.
	defer func() {
		// Handle panics: ensure workers are stopped even if panic occurs
		if r := recover(); r != nil {
			log.Printf("Panic recovered in main: %v", r)
			// Re-panic after cleanup
			defer panic(r)
		}

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := acpRuntime.Shutdown(shutdownCtx); err != nil {
			log.Printf("Warning: failed to shutdown ACP runtime: %v", err)
		}
	}()

	// Register session commands (needs active session).
	sessionCmdDeps := &repl.SessionCommandDeps{
		SessionRepo:   sessionRepo,
		ProjectRepo:   projectRepo,
		TaskRepo:      taskRepo,
		ActiveSession: activeSession,
		ACPRegistry:   acpRegistry,
	}
	if err := repl.RegisterSessionCommands(cmdRegistry, sessionCmdDeps); err != nil {
		return fmt.Errorf("register session commands: %w", err)
	}

	// Register project commands (Story 2.0).
	projectCmdDeps := &repl.ProjectCommandDeps{
		ProjectRepo:      projectRepo,
		SessionRepo:      sessionRepo,
		PermissionRepo:   permissionRepo,
		ActiveSession:    activeSession,
		ProviderRegistry: providerRegistry,
	}
	if err := repl.RegisterProjectCommands(cmdRegistry, projectCmdDeps); err != nil {
		return fmt.Errorf("register project commands: %w", err)
	}

	// Register mode commands.
	modeCmdDeps := &repl.ModeCommandDeps{
		ActiveSession: activeSession,
	}
	if err := repl.RegisterModeCommands(cmdRegistry, modeCmdDeps); err != nil {
		return fmt.Errorf("register mode commands: %w", err)
	}

	// Register verbosity commands.
	verbosityCmdDeps := &repl.VerbosityCommandDeps{
		ActiveSession: activeSession,
	}
	if err := repl.RegisterVerbosityCommands(cmdRegistry, verbosityCmdDeps); err != nil {
		return fmt.Errorf("register verbosity commands: %w", err)
	}

	// Register daemon commands (/tasks, /status, /approve, /deny).
	// CUIAdapter is passed from AdapterSet - only set in daemon mode.
	daemonCmdDeps := &repl.DaemonCommandDeps{
		TaskRepo:      taskRepo,
		WorkflowRepo:  workflowRepo,
		ActiveSession: activeSession,
		ACPRegistry:   acpRegistry,
		CUIAdapter:    adapters.CUI, // nil in CLI mode, set in daemon mode
	}
	if err := repl.RegisterDaemonCommands(cmdRegistry, daemonCmdDeps); err != nil {
		return fmt.Errorf("register daemon commands: %w", err)
	}

	// Register ACP commands with the full Runtime (Story 3.12).
	acpCmdDeps := &repl.ACPCommandDeps{
		ActiveSession:  activeSession,
		TaskRepo:       taskRepo,
		AgentRepo:      agentRepo,
		UIMediator:     mediator,
		BufferRegistry: bufferRegistry,
		ACPRuntime:     acpRuntime,
	}
	if err := repl.RegisterACPCommands(cmdRegistry, acpCmdDeps); err != nil {
		return fmt.Errorf("register acp commands: %w", err)
	}

	// Register provider commands (Story 3.22).
	providerCmdDeps := &repl.ProviderCommandDeps{
		ProviderRegistry:    providerRegistry,
		DefaultProviderName: systemProviderName,
	}
	if err := repl.RegisterProviderCommands(cmdRegistry, providerCmdDeps); err != nil {
		return fmt.Errorf("register provider commands: %w", err)
	}

	// Step 12: Start heartbeat goroutine if we have an active session.
	if activeSession.Session != nil {
		heartbeat := orchestrator.NewHeartbeat(sessionRepo, activeSession)
		go heartbeat.Start(ctx)
	}

	// Step 12b: Start scheduler goroutine if we have an active session and project.
	// The scheduler continuously pulls pending tasks and dispatches them to the ACP worker.
	// It will exit when the context is cancelled (SIGINT/SIGTERM).
	var scheduler *orchestrator.Scheduler
	if activeSession.Session != nil && activeSession.Project != nil {
		scheduler = orchestrator.NewSchedulerWithConfig(orchestrator.SchedulerConfig{
			TaskRepo:      taskRepo,
			WorkflowRepo:  workflowRepo,
			Executor:      orchestrator.NewExecutor(nil, taskRepo, agentRepo, mediator),
			ACPRegistry:   acpRegistry,
			ACPRuntime:    acpRuntime,
			ActiveSession: activeSession,
			AdvanceCh:     nil, // Will be wired from Runtime.advanceCh in a future story
		})
		go func() {
			if err := scheduler.Run(ctx); err != nil && ctx.Err() == nil {
				log.Printf("Warning: scheduler exited with error: %v", err)
			}
		}()
	}

	// Step 13: Create dispatcher wrapper for REPL loop.
	wrapper := &dispatcherWrapper{
		dispatcher: dispatcher,
		session:    activeSession,
	}

	// Step 13: Start REPL loop.
	fmt.Println("CodeMint - AI-powered coding assistant")
	fmt.Printf("Version: %s (commit: %s)\n", version, commit)
	fmt.Println("Type /help for available commands, /exit to quit.")
	fmt.Println()

	// Display session status.
	fmt.Println(loadResult.Message)
	fmt.Println()

	// The REPL loop handles shutdown via ErrShutdownGracefully.
	err = repl.Loop(ctx, wrapper, os.Stdin, os.Stderr)

	// Handle graceful shutdown.
	if errors.Is(err, registry.ErrShutdownGracefully) {
		fmt.Fprintln(os.Stderr, "Shutting down gracefully...")
		return err
	}

	// Context canceled (SIGINT/SIGTERM).
	if ctx.Err() != nil {
		fmt.Fprintln(os.Stderr, "\nReceived shutdown signal, exiting...")
		return nil
	}

	return err
}

// parseFlags parses command-line flags and returns the configuration.
func parseFlags() config {
	var cfg config

	flag.BoolVar(&cfg.showVersion, "version", false, "Print version information and exit")
	flag.BoolVar(&cfg.showVersion, "v", false, "Print version information and exit (shorthand)")
	flag.BoolVar(&cfg.showHelp, "help", false, "Print usage information and exit")
	flag.BoolVar(&cfg.showHelp, "h", false, "Print usage information and exit (shorthand)")

	defaultConfig := filepath.Join(xdg.ConfigDir(), "config.yaml")
	flag.StringVar(&cfg.configPath, "config", defaultConfig, "Path to configuration file")
	flag.StringVar(&cfg.configPath, "c", defaultConfig, "Path to configuration file (shorthand)")

	flag.StringVar(&cfg.dbPath, "db", "", "Override database path (default: "+xdg.DatabasePath()+")")
	flag.StringVar(&cfg.mode, "mode", "cli", "Client mode: cli, daemon, or hybrid (cli + daemon adapters together)")
	flag.BoolVar(&cfg.withAssistant, "with-assistant", true, "Enable System Assistant for freeform text queries")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: codemint [options]\n\n")
		fmt.Fprintf(os.Stderr, "CodeMint is an AI-powered coding assistant.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()
	return cfg
}

// parseClientMode validates and converts the mode string to ClientMode.
func parseClientMode(mode string) (registry.ClientMode, error) {
	switch mode {
	case "cli":
		return registry.ClientModeCLI, nil
	case "daemon":
		return registry.ClientModeDaemon, nil
	case "hybrid":
		return registry.ClientModeHybrid, nil
	default:
		return "", fmt.Errorf("invalid mode %q: must be 'cli', 'daemon', or 'hybrid'", mode)
	}
}

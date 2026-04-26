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

	appconfig "codemint.kanthorlabs.com/internal/config"
	"codemint.kanthorlabs.com/internal/db"
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
	showVersion bool
	showHelp    bool
	configPath  string
	dbPath      string
	mode        string
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

	// Step 6: Seed system agents.
	if err := agentRepo.EnsureSystemAgents(ctx); err != nil {
		return fmt.Errorf("seed system agents: %w", err)
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

	var workflowReg *workflow.WorkflowRegistry
	if len(appCfg.Workflows) > 0 {
		workflowReg, err = workflow.LoadFromConfig(appCfg)
		if err != nil {
			return fmt.Errorf("load workflow registry: %w", err)
		}
		log.Printf("Loaded %d workflow(s) from config", workflowReg.Len())
	}

	// Step 10: Create dispatcher (no system assistant yet - EPIC-02).
	dispatcher := orchestrator.NewDispatcher(cmdRegistry, mediator, nil, workflowReg)

	// Step 11: Create active session (global mode by default).
	activeSession := &orchestrator.ActiveSession{
		ClientMode: clientMode,
		IsGlobal:   true,
		Project:    nil,
		Session:    nil,
	}

	// Step 12: Create dispatcher wrapper for REPL loop.
	wrapper := &dispatcherWrapper{
		dispatcher: dispatcher,
		session:    activeSession,
	}

	// Step 13: Start REPL loop.
	fmt.Println("CodeMint - AI-powered coding assistant")
	fmt.Printf("Version: %s (commit: %s)\n", version, commit)
	fmt.Println("Type /help for available commands, /exit to quit.")
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

	// Suppress unused variable warnings during development.
	_ = taskRepo

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
	flag.StringVar(&cfg.mode, "mode", "cli", "Client mode: cli or daemon")

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
	default:
		return "", fmt.Errorf("invalid mode %q: must be 'cli' or 'daemon'", mode)
	}
}

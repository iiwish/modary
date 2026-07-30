package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"modary/core/action"
	"modary/core/config"
	"modary/core/module"
	"modary/internal/app"
	"modary/internal/transport/httpapi"
)

var processStartedAt = time.Now()

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return serve(nil)
	}
	switch args[0] {
	case "serve":
		return serve(args[1:])
	case "verify":
		return verify()
	case "generate":
		return generate()
	case "build":
		return build(args[1:])
	case "action":
		return runAction(args[1:], os.Stdout)
	case "version", "--version", "-v":
		fmt.Println("modary 0.1.0-f0")
		return nil
	case "help", "--help", "-h":
		printHelp()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printHelp() {
	fmt.Print(`Modary F0

Usage:
  modary serve [--listen address] [--db path]
  modary verify
  modary generate
  modary build [--output path]
  modary action catalog [--output path]
  modary action run <action-id> --actor <id> --input <json-file> [--preview]
`)
}

func serve(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	cfg := config.FromEnvironment()
	flags.StringVar(&cfg.ListenAddress, "listen", cfg.ListenAddress, "listen address")
	flags.StringVar(&cfg.DatabasePath, "db", cfg.DatabasePath, "SQLite database path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := config.ValidateForServe(cfg); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	application, err := app.Bootstrap(ctx, cfg)
	if err != nil {
		return err
	}
	defer application.Close()
	server := &http.Server{
		Addr: cfg.ListenAddress, Handler: httpapi.New(application),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second,
	}
	listener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.ListenAddress, err)
	}
	application.StartupDuration = time.Since(processStartedAt)
	result := make(chan error, 1)
	go func() {
		slog.Info("Modary is ready", "address", "http://"+listener.Addr().String(), "database", cfg.DatabasePath, "startup_ms", application.StartupDuration.Milliseconds())
		result <- server.Serve(listener)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return server.Shutdown(shutdownCtx)
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func projectFromCWD() (module.Project, error) {
	root, err := findRoot()
	if err != nil {
		return module.Project{}, err
	}
	return module.LoadProject(root)
}

func verify() error {
	project, err := projectFromCWD()
	if err != nil {
		return err
	}
	fmt.Printf("verified app %s: %d modules, %d dependency edges\n", project.App.App.ID, len(project.Manifests), len(project.Graph.Edges))
	fmt.Printf("startup order: %s\n", strings.Join(project.Graph.Order, " -> "))
	return nil
}

func generate() error {
	project, err := projectFromCWD()
	if err != nil {
		return err
	}
	if err := project.Generate(); err != nil {
		return err
	}
	fmt.Println("generated static module registry, UI routes, action catalog, and module graph")
	return nil
}

func build(args []string) error {
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	output := flags.String("output", "dist/modary-rulary", "release output path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	project, err := projectFromCWD()
	if err != nil {
		return err
	}
	if err := project.Generate(); err != nil {
		return err
	}
	commands := [][]string{
		{"pnpm", "--filter", "@modary/console", "build"},
		{"go", "test", "./..."},
		{"go", "build", "-trimpath", "-ldflags=-s -w", "-o", *output, "./cmd/modary"},
	}
	for _, command := range commands {
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Dir = project.Root
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if err := os.MkdirAll(filepath.Join(project.Root, filepath.Dir(*output)), 0o755); err != nil {
			return err
		}
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("run %s: %w", strings.Join(command, " "), err)
		}
	}
	fmt.Printf("built %s\n", *output)
	return nil
}

func runAction(args []string, output io.Writer) error {
	if len(args) > 0 && args[0] == "catalog" {
		return exportActionCatalog(args[1:])
	}
	if len(args) < 2 || args[0] != "run" {
		return fmt.Errorf("usage: modary action catalog [--output path] | modary action run <action-id> [flags]")
	}
	actionID := args[1]
	flags := flag.NewFlagSet("action run", flag.ContinueOnError)
	actorID := flags.String("actor", "", "actor id")
	inputPath := flags.String("input", "", "JSON input file")
	previewOnly := flags.Bool("preview", false, "create a preview plan")
	planHash := flags.String("plan", "", "preview plan hash")
	idempotencyKey := flags.String("idempotency-key", "", "idempotency key")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if *actorID == "" || *inputPath == "" {
		return fmt.Errorf("--actor and --input are required")
	}
	input, err := os.ReadFile(*inputPath)
	if err != nil {
		return err
	}
	cfg := config.FromEnvironment()
	application, err := app.Bootstrap(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer application.Close()
	actor, err := application.Identity.ResolveByID(context.Background(), *actorID)
	if err != nil {
		return err
	}
	request := action.Request{
		Actor: actor, Channel: "cli", ActionID: actionID, WorkspaceID: actor.WorkspaceID,
		Input: input, PlanHash: *planHash, IdempotencyKey: *idempotencyKey,
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if *previewOnly {
		preview, err := application.Runtime.Preview(context.Background(), request)
		if err != nil {
			return err
		}
		return encoder.Encode(preview)
	}
	result, err := application.Runtime.Execute(context.Background(), request)
	if err != nil {
		return err
	}
	var value any
	_ = json.Unmarshal(result.Data, &value)
	return encoder.Encode(value)
}

func exportActionCatalog(args []string) error {
	flags := flag.NewFlagSet("action catalog", flag.ContinueOnError)
	output := flags.String("output", "", "write the descriptor catalog to a JSON file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	application, err := app.Bootstrap(context.Background(), config.FromEnvironment())
	if err != nil {
		return err
	}
	defer application.Close()
	descriptors := make([]action.Descriptor, 0)
	for _, registered := range application.Registry.List() {
		descriptors = append(descriptors, registered.Descriptor)
	}
	data, err := json.MarshalIndent(map[string]any{
		"schema_version": "modary.action-catalog/v1alpha1",
		"actions":        descriptors,
	}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if *output == "" {
		_, err = os.Stdout.Write(data)
		return err
	}
	path, err := filepath.Abs(*output)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %d action descriptors to %s\n", len(descriptors), path)
	return nil
}

func findRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(current, "modary.yaml")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("modary.yaml was not found")
		}
		current = parent
	}
}

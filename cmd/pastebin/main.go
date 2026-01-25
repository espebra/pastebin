package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/espebra/pastebin/internal/cleanup"
	"github.com/espebra/pastebin/internal/config"
	"github.com/espebra/pastebin/internal/handlers"
	"github.com/espebra/pastebin/internal/storage"
	"github.com/espebra/pastebin/web"
)

func main() {
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *versionFlag {
		printVersion()
		os.Exit(0)
	}

	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

// Version can be set via ldflags for release builds (e.g., -X main.Version=v1.0.0)
var Version = ""

// getVersionInfo returns version, commit hash, and modified status
func getVersionInfo() (version, commit, modified string) {
	version = Version
	commit = "unknown"
	modified = ""

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				commit = setting.Value
				if len(commit) > 7 {
					commit = commit[:7]
				}
			case "vcs.modified":
				if setting.Value == "true" {
					modified = "-dirty"
				}
			}
		}
		if version == "" {
			v := info.Main.Version
			// Use module version only if it's a proper semver tag, not a pseudo-version
			// Pseudo-versions look like v0.0.0-20060102150405-abcdef123456
			isPseudo := strings.HasPrefix(v, "v0.0.0-") || strings.Contains(v, "-0.")
			if v != "" && v != "(devel)" && !isPseudo {
				version = v
			} else {
				version = "dev"
			}
		}
	}

	return version, commit, modified
}

func printVersion() {
	version, commit, modified := getVersionInfo()
	fmt.Printf("pastebin %s (commit: %s%s)\n", version, commit, modified)
}

func run() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Configure structured logging
	configureLogger(cfg.LogFormat, cfg.LogLevel)

	// Log version information at startup
	version, commit, modified := getVersionInfo()
	slog.Info("starting pastebin", "version", version, "commit", commit+modified)

	// Log configuration (without secrets)
	slog.Info("configuration loaded",
		"host", cfg.Host,
		"port", cfg.Port,
		"s3_endpoint", cfg.S3Endpoint,
		"s3_region", cfg.S3Region,
		"s3_bucket", cfg.S3Bucket,
		"s3_use_ssl", cfg.S3UseSSL,
		"cleanup_interval", cfg.CleanupInterval.String(),
		"max_paste_size", cfg.MaxPasteSize,
		"default_ttl", cfg.DefaultTTL.String(),
	)

	// Initialize S3 storage
	ctx := context.Background()
	store, err := storage.New(
		ctx,
		cfg.S3Endpoint,
		cfg.S3Region,
		cfg.S3Bucket,
		cfg.AWSAccessKey,
		cfg.AWSSecretKey,
		cfg.S3UseSSL,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Extract static subdirectory from embedded FS
	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		return fmt.Errorf("failed to get static fs: %w", err)
	}

	// Create HTTP handler
	handler, err := handlers.New(cfg, store, web.Templates)
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	// Set up router
	mux := http.NewServeMux()
	wrappedHandler := handler.RegisterRoutes(mux, staticFS)

	// Create server
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           wrappedHandler,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
	}

	// Start cleanup goroutine
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	cleaner := cleanup.New(store, cfg.CleanupInterval)
	go cleaner.Start(cleanupCtx)

	// Handle graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("starting server", "address", addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case sig := <-shutdown:
		slog.Info("received shutdown signal", "signal", sig.String())
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	slog.Info("server stopped")
	return nil
}

// configureLogger sets up the default slog logger
func configureLogger(format, level string) {
	opts := &slog.HandlerOptions{
		Level: parseLogLevel(level),
	}

	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}

// parseLogLevel converts a string log level to slog.Level
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Command server is Trakka's entry point: it loads configuration, opens
// the database, wires up the HTTP handler, and runs the server with a
// graceful shutdown path that closes the database connection cleanly.
package main

import (
	"context"
	"errors"
	"flag"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"trakka/internal/auth"
	"trakka/internal/config"
	"trakka/internal/db"
	"trakka/internal/handlers"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe the local /healthz endpoint and exit (used by HEALTHCHECK)")
	flag.Parse()

	cfg := config.Load()

	if *healthcheck {
		os.Exit(probeHealthz(cfg.Port))
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := cfg.Validate(); err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		logger.Error("opening database", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := database.Close(); err != nil {
			logger.Error("closing database", "error", err)
		}
	}()

	var oidcClient *auth.OIDCClient
	if cfg.OIDCEnabled() {
		oidcClient, err = auth.NewOIDCClient(context.Background(), cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCClientSecret, cfg.BaseURL+"/auth/oidc/callback")
		if err != nil {
			logger.Error("oidc discovery failed", "error", err)
			os.Exit(1)
		}
	}
	authService := auth.NewService(database, oidcClient, cfg.SessionTTL, cfg.SessionCookieSecure)

	loginTemplate, err := template.ParseFiles(filepath.Join(cfg.TemplatesDir, "login.html"))
	if err != nil {
		logger.Error("parsing login template", "error", err)
		os.Exit(1)
	}

	app := &handlers.Application{
		DB:            database,
		StaticDir:     cfg.StaticDir,
		Logger:        logger,
		Auth:          authService,
		LoginTemplate: loginTemplate,
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrs := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", srv.Addr, "db_path", cfg.DBPath, "static_dir", cfg.StaticDir)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrs <- err
			return
		}
		serverErrs <- nil
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrs:
		if err != nil {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}

	case <-stop:
		logger.Info("shutdown signal received")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
		}
		// database.Close() runs via the deferred call above once main
		// returns, after in-flight requests have drained.
	}
}

// probeHealthz is invoked as `trakka -healthcheck` from the container
// HEALTHCHECK. Doing it in-process avoids shipping curl/wget in the
// runtime image, keeping the image (and its attack surface) smaller.
func probeHealthz(port string) int {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

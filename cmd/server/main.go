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
	"trakka/internal/settings"
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

	resolvedSettings, err := settings.Resolve(context.Background(), database, cfg)
	if err != nil {
		logger.Error("resolving system settings", "error", err)
		os.Exit(1)
	}

	var oidcClient *auth.OIDCClient
	if resolvedSettings.OIDCEnabled {
		switch {
		case resolvedSettings.OIDCIssuer == "" || resolvedSettings.OIDCClientID == "" || resolvedSettings.OIDCClientSecret == "":
			logger.Warn("oidc is marked enabled but incompletely configured; starting with OIDC disabled until fixed via PATCH /api/v1/admin/settings")
		case cfg.BaseURL == "":
			logger.Warn("oidc is enabled but BASE_URL is not set; starting with OIDC disabled")
		default:
			// Unlike the env-var-only OIDC config this replaces (which still
			// fails startup outright via cfg.Validate() below on a broken
			// all-or-nothing env setup), a DB-driven OIDC config that was valid
			// when an admin saved it can still fail discovery later purely
			// because the IdP is temporarily unreachable. Crashing local-auth
			// availability along with it on every restart until the IdP comes
			// back would be worse than starting up with OIDC login simply
			// unavailable — so this logs and continues rather than exiting.
			oidcClient, err = auth.NewOIDCClient(context.Background(), resolvedSettings.OIDCIssuer, resolvedSettings.OIDCClientID, resolvedSettings.OIDCClientSecret, cfg.BaseURL+"/auth/oidc/callback")
			if err != nil {
				logger.Error("oidc discovery failed at startup; starting with OIDC disabled", "error", err)
				oidcClient = nil
			}
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
		Config:        cfg,
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// The periodic price-drop scan runs detached from any request, on its
	// own cancelable context, the same "never r.Context()" reasoning
	// scrapeProductInfo already follows — canceled only on shutdown, below.
	priceScanCtx, cancelPriceScan := context.WithCancel(context.Background())
	defer cancelPriceScan()
	if cfg.PriceCheckInterval > 0 {
		go runPriceAlertScanLoop(priceScanCtx, app, cfg.PriceCheckInterval, logger)
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

// runPriceAlertScanLoop periodically re-checks every eligible item's price
// for a better deal (see handlers.Application.RunPriceAlertScan), stopping
// once ctx is canceled during shutdown. It runs an initial scan right away
// rather than waiting a full interval for the first one, so a freshly
// deployed instance doesn't sit with an empty notification center for up to
// PRICE_CHECK_INTERVAL_HOURS before its first check. Runs in its own
// goroutine (see the call site in main), so this scan itself never delays
// server startup.
func runPriceAlertScanLoop(ctx context.Context, app *handlers.Application, interval time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("starting periodic price alert scan", "interval", interval)
	app.RunPriceAlertScan(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			app.RunPriceAlertScan(ctx)
		}
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

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/storage"
	"github.com/lieranderl/moviestracker-backend/internal/imdb"
	gcsstore "github.com/lieranderl/moviestracker-backend/internal/platform/gcs"
	"google.golang.org/api/idtoken"
)

type config struct {
	bucket          string
	object          string
	port            string
	refreshInterval time.Duration
	// refreshInvoker is the only identity allowed to call /internal/refresh.
	refreshInvoker string
}

// tokenValidator verifies a Google-signed ID token minted for audience.
type tokenValidator func(ctx context.Context, token, audience string) (*idtoken.Payload, error)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Getenv); err != nil {
		slog.Error("IMDb service failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, getenv func(string) string) error {
	config, err := loadConfig(getenv)
	if err != nil {
		return err
	}
	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("create storage client: %w", err)
	}
	defer func() {
		if err := storageClient.Close(); err != nil {
			slog.Error("close storage client", "error", err)
		}
	}()
	source := gcsstore.RatingsSource{Bucket: config.bucket, Client: storageClient, Object: config.object}
	manager := imdb.NewManager(imdb.MinSnapshotRows)
	if err := manager.Refresh(ctx, source); err != nil {
		return fmt.Errorf("load initial IMDb index: %w", err)
	}

	mux := http.NewServeMux()
	handler := imdb.NewHandler(manager.Lookup)
	// Freshness runs on the request path, not on a timer: this service has CPU
	// allocated only while serving, so a background ticker is not reliably
	// scheduled. Probes are registered outside the middleware so they never
	// trigger a storage call.
	refresher := imdb.NewRefresher(manager, source, config.refreshInterval)
	fresh := refresher.Middleware(handler)
	mux.Handle("GET /getimdb", fresh)
	mux.Handle("GET /", fresh)
	if config.refreshInvoker != "" {
		mux.Handle("POST /internal/refresh", requireInvoker(imdb.NewRefreshHandler(refresher), idtoken.Validate, config.refreshInvoker))
	} else {
		slog.Warn("IMDB_REFRESH_INVOKER is unset; /internal/refresh is disabled", "event", "imdb_refresh_disabled")
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if manager.Generation() == 0 {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ready"))
	})
	server := &http.Server{
		Addr:              ":" + config.port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServe() }()
	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown IMDb service: %w", err)
		}
		return nil
	}
}

func loadConfig(getenv func(string) string) (config, error) {
	bucket := strings.TrimSpace(getenv("IMDB_BUCKET"))
	if bucket == "" {
		return config{}, errors.New("IMDB_BUCKET is required")
	}
	object := strings.TrimSpace(getenv("IMDB_OBJECT"))
	if object == "" {
		object = "title.ratings.tsv.gz"
	}
	port := strings.TrimSpace(getenv("PORT"))
	if port == "" {
		port = "8080"
	}
	interval := 15 * time.Minute
	if parsed, err := time.ParseDuration(strings.TrimSpace(getenv("IMDB_REFRESH_INTERVAL"))); err == nil && parsed > 0 {
		interval = parsed
	}
	return config{
		bucket:          bucket,
		object:          object,
		port:            port,
		refreshInterval: interval,
		refreshInvoker:  strings.TrimSpace(getenv("IMDB_REFRESH_INVOKER")),
	}, nil
}

// requireInvoker admits only Google ID tokens minted for this service's URL
// and issued to the email service account. The service has Cloud Run invoker
// IAM disabled so installed clients can read ratings anonymously; the refresh
// endpoint bypasses the refresh interval and must not be public with it.
func requireInvoker(next http.Handler, validate tokenValidator, email string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// The downloader mints the token for the service URL it calls, which
		// Cloud Run delivers as the Host header.
		payload, err := validate(r.Context(), token, "https://"+r.Host)
		if err != nil {
			slog.Warn("reject IMDb refresh token", "event", "imdb_refresh_rejected", "error", err)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		caller, _ := payload.Claims["email"].(string)
		verified, _ := payload.Claims["email_verified"].(bool)
		if caller != email || !verified {
			slog.Warn("reject IMDb refresh caller", "event", "imdb_refresh_rejected", "caller", caller)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

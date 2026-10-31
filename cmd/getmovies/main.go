package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/lieranderl/moviestracker-backend/internal/catalog"
	firestorestore "github.com/lieranderl/moviestracker-backend/internal/platform/firestore"
)

const defaultJobTimeout = 10 * time.Minute

type config struct {
	collection          catalog.Collection
	database            string
	enricherConcurrency int
	project             string
	timeout             time.Duration
	tmdbAPIKey          string
	urls                []string
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv); err != nil {
		slog.Error("catalog job failed", "error", err)
		os.Exit(1)
	}
}

func run(parent context.Context, args []string, getenv func(string) string) error {
	config, err := loadConfig(args, getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, config.timeout)
	defer cancel()

	client, err := firestore.NewClientWithDatabase(ctx, config.project, config.database)
	if err != nil {
		return fmt.Errorf("create Firestore client: %w", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			slog.Error("close Firestore client", "error", err)
		}
	}()

	httpClient := &http.Client{Timeout: 30 * time.Second}
	runner := catalog.Runner{
		Tracker:    catalog.NewRutorClient(httpClient),
		Enricher:   catalog.NewTMDBClient(httpClient, strings.TrimSpace(getenv("TMDB_BASE_URL")), config.tmdbAPIKey),
		Repository: firestorestore.New(client),
	}
	result, err := runner.Run(ctx, catalog.RunConfig{
		Collection:          config.collection,
		EnricherConcurrency: config.enricherConcurrency,
		URLs:                config.urls,
	})
	if err != nil {
		return fmt.Errorf("run catalog pipeline: %w", err)
	}
	// The event field is matched by the log-based metric in
	// infra/prod/monitoring.tf; update both atomically if the contract changes.
	slog.Info("catalog pipeline completed",
		"event", "catalog_pipeline_completed",
		"collection", config.collection,
		"fetched", result.Fetched,
		"rejected_parsing", result.RejectedParsing,
		"grouped", result.Grouped,
		"resolved", result.Resolved,
		"unresolved", result.Unresolved,
		"failed", result.Failed,
		"stored", result.Stored,
		"written", result.Written,
	)
	return nil
}

func loadConfig(args []string, getenv func(string) string) (config, error) {
	if len(args) != 1 {
		return config{}, errors.New("usage: getmovies <latesttorrentsmovies|hdr10movies|dvmovies>")
	}
	collection := catalog.Collection(strings.TrimSpace(args[0]))
	if !collection.Valid() {
		return config{}, fmt.Errorf("unsupported collection %q", collection)
	}
	urlKey := map[catalog.Collection]string{
		catalog.CollectionLatest: "RUTOR_URLS",
		catalog.CollectionHDR10:  "RUTOR_HDR10_URLS",
		catalog.CollectionDV:     "RUTOR_DV_URLS",
	}[collection]
	urls := splitURLs(getenv(urlKey))
	if len(urls) == 0 {
		return config{}, fmt.Errorf("%s is required", urlKey)
	}
	project := firstConfigured(getenv("GCP_PROJECT"), getenv("GOOGLE_CLOUD_PROJECT"))
	if project == "" {
		return config{}, errors.New("GCP_PROJECT is required")
	}
	tmdbAPIKey := strings.TrimSpace(getenv("TMDBAPIKEY"))
	if tmdbAPIKey == "" {
		return config{}, errors.New("TMDBAPIKEY is required")
	}
	database := strings.TrimSpace(getenv("FIRESTORE_DATABASE"))
	if database == "" {
		database = "moviestracker"
	}
	timeout := defaultJobTimeout
	if parsed, err := time.ParseDuration(strings.TrimSpace(getenv("JOB_TIMEOUT"))); err == nil && parsed > 0 {
		timeout = parsed
	}
	enricherConcurrency := 10
	if raw := strings.TrimSpace(getenv("ENRICHER_CONCURRENCY")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 20 {
			return config{}, errors.New("ENRICHER_CONCURRENCY must be between 1 and 20")
		}
		enricherConcurrency = parsed
	}
	return config{collection: collection, database: database, enricherConcurrency: enricherConcurrency, project: project, timeout: timeout, tmdbAPIKey: tmdbAPIKey, urls: urls}, nil
}

func splitURLs(raw string) []string {
	result := make([]string, 0)
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func firstConfigured(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

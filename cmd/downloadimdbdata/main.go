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

const defaultDownloadTimeout = 5 * time.Minute

type config struct {
	bucket         string
	imdbServiceURL string
	indexObject    string
	object         string
	sourceURL      string
	timeout        time.Duration
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Getenv); err != nil {
		slog.Error("IMDb download job failed", "error", err)
		os.Exit(1)
	}
}

func run(parent context.Context, getenv func(string) string) error {
	config, err := loadConfig(getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, config.timeout)
	defer cancel()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("create storage client: %w", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			slog.Error("close storage client", "error", err)
		}
	}()

	archive, index, stats, err := imdb.DownloadRatings(ctx, &http.Client{Timeout: config.timeout}, config.sourceURL, imdb.MinSnapshotRows)
	if err != nil {
		return err
	}
	exportGeneration, err := (gcsstore.RatingsStore{Bucket: config.bucket, Client: client, Object: config.object}).Put(ctx, archive)
	if err != nil {
		return err
	}
	// The index is an accelerator: the service serves the export whenever no
	// index matches it, so an index failure is logged without failing the run.
	indexBytes, err := publishIndex(ctx, gcsstore.RatingsStore{
		Bucket:           config.bucket,
		Client:           client,
		ContentType:      "application/octet-stream",
		Object:           config.indexObject,
		SourceGeneration: exportGeneration,
	}, index)
	if err != nil {
		slog.Error("IMDb binary index not published; the service will serve the export",
			"event", "imdb_index_publish_failed",
			"error", err,
		)
	}
	if config.imdbServiceURL != "" {
		authenticatedClient, err := idtoken.NewClient(ctx, config.imdbServiceURL)
		if err != nil {
			return fmt.Errorf("create IMDb refresh identity client: %w", err)
		}
		if err := notifyRefresh(ctx, authenticatedClient, config.imdbServiceURL); err != nil {
			return err
		}
	}
	slog.Info("IMDb ratings updated",
		"event", "imdb_ratings_updated",
		"bytes", len(archive),
		"index_bytes", indexBytes,
		"index_object", config.indexObject,
		"object", config.object,
		"rows", stats.Rows(),
	)
	return nil
}

func publishIndex(ctx context.Context, store gcsstore.RatingsStore, index *imdb.Index) (int, error) {
	encoded, err := imdb.MarshalIndex(index)
	if err != nil {
		return 0, err
	}
	if _, err := store.Put(ctx, encoded); err != nil {
		return 0, err
	}
	return len(encoded), nil
}

func notifyRefresh(ctx context.Context, client *http.Client, serviceURL string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(serviceURL, "/")+"/internal/refresh", nil)
	if err != nil {
		return fmt.Errorf("build IMDb refresh request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("invoke IMDb refresh: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("invoke IMDb refresh: unexpected status %s", response.Status)
	}
	return nil
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
	indexObject := strings.TrimSpace(getenv("IMDB_INDEX_OBJECT"))
	if indexObject == "" {
		indexObject = "title.ratings.idx"
	}
	if indexObject == object {
		return config{}, errors.New("IMDB_INDEX_OBJECT must differ from IMDB_OBJECT")
	}
	sourceURL := strings.TrimSpace(getenv("IMDB_SOURCE_URL"))
	if sourceURL == "" {
		sourceURL = "https://datasets.imdbws.com/title.ratings.tsv.gz"
	}
	timeout := defaultDownloadTimeout
	if parsed, err := time.ParseDuration(strings.TrimSpace(getenv("JOB_TIMEOUT"))); err == nil && parsed > 0 {
		timeout = parsed
	}
	return config{
		bucket:         bucket,
		imdbServiceURL: strings.TrimRight(strings.TrimSpace(getenv("IMDB_SERVICE_URL")), "/"),
		indexObject:    indexObject,
		object:         object,
		sourceURL:      sourceURL,
		timeout:        timeout,
	}, nil
}

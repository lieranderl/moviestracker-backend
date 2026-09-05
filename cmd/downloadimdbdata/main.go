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
)

const defaultDownloadTimeout = 5 * time.Minute

type config struct {
	bucket    string
	object    string
	sourceURL string
	timeout   time.Duration
}

func main() {
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

	archive, err := imdb.DownloadRatings(ctx, &http.Client{Timeout: config.timeout}, config.sourceURL)
	if err != nil {
		return err
	}
	if err := (gcsstore.RatingsStore{Bucket: config.bucket, Client: client, Object: config.object}).Put(ctx, archive); err != nil {
		return err
	}
	slog.Info("IMDb ratings updated", "bytes", len(archive), "object", config.object)
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
	sourceURL := strings.TrimSpace(getenv("IMDB_SOURCE_URL"))
	if sourceURL == "" {
		sourceURL = "https://datasets.imdbws.com/title.ratings.tsv.gz"
	}
	timeout := defaultDownloadTimeout
	if parsed, err := time.ParseDuration(strings.TrimSpace(getenv("JOB_TIMEOUT"))); err == nil && parsed > 0 {
		timeout = parsed
	}
	return config{bucket: bucket, object: object, sourceURL: sourceURL, timeout: timeout}, nil
}

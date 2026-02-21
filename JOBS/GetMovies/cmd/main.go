package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/lieranderl/moviestracker-package/executor"
)

const (
	defaultJobTimeout = 10 * time.Minute
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv); err != nil {
		log.Fatalf("getmovies job failed: %v", err)
	}
}

func run(parent context.Context, args []string, getenv func(string) string) error {
	if len(args) < 2 {
		return errors.New("usage: getmovies <latesttorrentsmovies|hdr10movies|dvmovies> <mongo>")
	}

	dbname := args[0]
	dbtype := normalizeDBType(args[1])

	urls, err := getSourceURLs(dbname, getenv)
	if err != nil {
		return err
	}
	if len(urls) == 0 {
		return fmt.Errorf("no source URLs configured for %s", dbname)
	}

	tmdbAPIKey := strings.TrimSpace(getenv("TMDBAPIKEY"))
	if tmdbAPIKey == "" {
		return errors.New("TMDBAPIKEY is required")
	}

	timeout := parseJobTimeout(getenv("JOB_TIMEOUT"))
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	log.Printf("Start %s !", dbname)
	start := time.Now()

	var pipelineErr error
	switch dbtype {
	case "mongo":
		mongoURI := strings.TrimSpace(getenv("MONGO_URI"))
		if mongoURI == "" {
			return errors.New("MONGO_URI is required for mongo mode")
		}
		envVars := executor.InitVars(urls, tmdbAPIKey).WithMongo(mongoURI)
		pipelineErr = executor.Init(*envVars).
			RunTrackersSearchPipeline(true).
			ConvertTorrentsToMovieShort().
			Tmdb().
			SaveToMongo(dbname).
			HandleErrors()
	case "firebase":
		return errors.New("firebase mode is no longer supported in moviestracker-package v1.2.0")
	default:
		return fmt.Errorf("unsupported db type: %s", dbtype)
	}

	if pipelineErr != nil {
		return fmt.Errorf("pipeline execution failed: %w", pipelineErr)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("job timeout reached: %w", err)
	}

	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)
	return nil
}

func normalizeDBType(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}

func getSourceURLs(dbname string, getenv func(string) string) ([]string, error) {
	var key string
	switch dbname {
	case "latesttorrentsmovies":
		key = "RUTOR_URLS"
	case "hdr10movies":
		key = "RUTOR_HDR10_URLS"
	case "dvmovies":
		key = "RUTOR_DV_URLS"
	default:
		return nil, fmt.Errorf("unsupported db name: %s", dbname)
	}

	return splitAndTrimURLs(getenv(key)), nil
}

func splitAndTrimURLs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	urls := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			urls = append(urls, trimmed)
		}
	}
	return urls
}

func parseJobTimeout(raw string) time.Duration {
	if strings.TrimSpace(raw) == "" {
		return defaultJobTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultJobTimeout
	}
	return d
}

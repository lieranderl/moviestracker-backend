package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lieranderl/moviestracker-package/executor"
)

const (
	defaultPort         = "8080"
	searchTimeout       = 25 * time.Second
	readHeaderTimeout   = 5 * time.Second
	readTimeout         = 10 * time.Second
	writeTimeout        = 30 * time.Second
	idleTimeout         = 60 * time.Second
	maxShutdownDuration = 10 * time.Second
)

type pipelineResult struct {
	Torrents any
	Count    int
}

var searchTorrents = func(ctx context.Context, urls []string, tmdbAPIKey string, isMovie bool) (pipelineResult, error) {
	_ = ctx
	pipeline := executor.Init(*executor.InitVars(urls, tmdbAPIKey))
	if err := pipeline.RunTrackersSearchPipeline(isMovie).HandleErrors(); err != nil {
		return pipelineResult{}, err
	}

	torrents := pipeline.GetTorrents()
	return pipelineResult{
		Torrents: torrents,
		Count:    len(torrents),
	}, nil
}

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func run(parent context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleRequest)
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /readyz", handleReadyz)

	server := &http.Server{
		Addr:              ":" + getPort(),
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("starting server on %s", server.Addr)
		errCh <- server.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), maxShutdownDuration)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

func getPort() string {
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		return port
	}
	return defaultPort
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func handleReadyz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	moviename := strings.TrimSpace(r.URL.Query().Get("MovieName"))
	year := strings.TrimSpace(r.URL.Query().Get("Year"))
	ismovie := strings.TrimSpace(r.URL.Query().Get("isMovie"))
	tmdbAPIKey := r.URL.Query().Get("key")
	if tmdbAPIKey == "" {
		tmdbAPIKey = os.Getenv("TMDBAPIKEY")
	}

	if moviename == "" {
		http.Error(w, "MovieName is required", http.StatusBadRequest)
		return
	}

	if year != "" && !isValidYear(year) {
		http.Error(w, "Year must be a 4-digit number", http.StatusBadRequest)
		return
	}

	isboolmovie, err := parseIsMovie(ismovie)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	urls, err := buildTrackerURLs(moviename, year)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), searchTimeout)
	defer cancel()

	result, err := searchTorrents(ctx, urls, tmdbAPIKey, isboolmovie)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)
	log.Printf("Number of torrents returned: %d", result.Count)

	if result.Count == 0 {
		http.Error(w, "No torrents found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result.Torrents); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func parseIsMovie(raw string) (bool, error) {
	if raw == "" {
		return true, nil
	}

	isMovie, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid isMovie value: %q", raw)
	}
	return isMovie, nil
}

func isValidYear(year string) bool {
	if len(year) != 4 {
		return false
	}
	_, err := strconv.Atoi(year)
	return err == nil
}

func buildTrackerURLs(movieName, year string) ([]string, error) {
	rutorTemplate := os.Getenv("RUTOR_SEARCH_URL")
	kzTemplate := os.Getenv("KZ_SEARCH_URL")
	if rutorTemplate == "" || kzTemplate == "" {
		return nil, errors.New("missing tracker URL templates")
	}

	rawURLs := []string{
		fmt.Sprintf(rutorTemplate, movieName, year),
		fmt.Sprintf(kzTemplate, movieName, year),
	}

	urls := make([]string, 0, len(rawURLs))
	for _, raw := range rawURLs {
		escaped, err := normalizeTrackerURL(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid tracker url %q: %w", raw, err)
		}
		urls = append(urls, escaped)
	}
	return urls, nil
}

func normalizeTrackerURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}

	if u.RawQuery != "" {
		values, err := url.ParseQuery(u.RawQuery)
		if err != nil {
			return "", err
		}
		u.RawQuery = values.Encode()
	}

	return u.String(), nil
}

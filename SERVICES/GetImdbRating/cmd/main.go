package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"moviestracker.net/getimdbrating/imdbrating"
)

const (
	defaultPort         = "8080"
	readHeaderTimeout   = 5 * time.Second
	readTimeout         = 10 * time.Second
	writeTimeout        = 15 * time.Second
	idleTimeout         = 60 * time.Second
	maxShutdownDuration = 10 * time.Second
)

var imdbIDPattern = regexp.MustCompile(`^tt[0-9]+$`)

var getIMDbRatingByID = func(imdbID string) ([]byte, error) {
	dataPath := strings.TrimSpace(os.Getenv("IMDB_DATA_PATH"))
	if dataPath != "" {
		return imdbrating.GetImdbRatingByIdFromFile(dataPath, imdbID)
	}
	return imdbrating.GetImdbRatingById(imdbID)
}

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func run(parent context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleGetIMDbRating)
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

func handleGetIMDbRating(w http.ResponseWriter, r *http.Request) {
	imdbID := strings.TrimSpace(r.URL.Query().Get("imdb_id"))
	if imdbID == "" {
		http.Error(w, "imdb_id is required", http.StatusBadRequest)
		return
	}
	if !imdbIDPattern.MatchString(imdbID) {
		http.Error(w, "invalid imdb_id format", http.StatusBadRequest)
		return
	}

	payload, err := getIMDbRatingByID(imdbID)
	if err != nil {
		if errors.Is(err, imdbrating.ErrRatingNotFound) {
			http.Error(w, "imdb rating not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load imdb rating", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestHandleRequest_ReturnsOutput_ForTheMummyReturnsQuery(t *testing.T) {
	t.Setenv("RUTOR_SEARCH_URL", "http://rutor.is/search/0/0/300/0/%s %s (2160p|1080p)")
	t.Setenv("KZ_SEARCH_URL", "http://kinozal.tv/browse.php?s=%s (1080p|2160p)&g=3&c=0&v=0&d=%s&w=0&t=0&f=0")
	t.Setenv("TMDBAPIKEY", "env-default-key")

	var gotURLs []string
	var gotKey string
	var gotIsMovie bool

	original := searchTorrents
	searchTorrents = func(_ context.Context, urls []string, tmdbAPIKey string, isMovie bool) (pipelineResult, error) {
		gotURLs = urls
		gotKey = tmdbAPIKey
		gotIsMovie = isMovie
		return pipelineResult{
			Torrents: []map[string]any{
				{
					"name":    "The Mummy Returns (2001) 1080p",
					"tracker": "rutor",
				},
			},
			Count: 1,
		}, nil
	}
	t.Cleanup(func() {
		searchTorrents = original
	})

	req := httptest.NewRequest(http.MethodGet, "/?MovieName=The+Mummy+Returns&Year=2001&isMovie=true&key=AIzaSyBEEuq4hIWsG7Rfe8pOelW8dne1byEhgw8", nil)
	rr := httptest.NewRecorder()

	handleRequest(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d, body=%q", http.StatusOK, rr.Code, rr.Body.String())
	}

	if gotKey != "AIzaSyBEEuq4hIWsG7Rfe8pOelW8dne1byEhgw8" {
		t.Fatalf("expected query key to be used, got %q", gotKey)
	}

	if !gotIsMovie {
		t.Fatal("expected isMovie=true to be passed as boolean true")
	}

	wantRutor := "http://rutor.is/search/0/0/300/0/The%20Mummy%20Returns%202001%20%282160p%7C1080p%29"
	wantKZ := "http://kinozal.tv/browse.php?c=0&d=2001&f=0&g=3&s=The+Mummy+Returns+%281080p%7C2160p%29&t=0&v=0&w=0"
	if len(gotURLs) != 2 || gotURLs[0] != wantRutor || gotURLs[1] != wantKZ {
		t.Fatalf("unexpected urls: %#v", gotURLs)
	}

	var out []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not valid json: %v", err)
	}
	if len(out) != 1 || out[0]["name"] != "The Mummy Returns (2001) 1080p" {
		t.Fatalf("unexpected response payload: %#v", out)
	}
}

func TestHandleRequest_EmptyMovieName_ReturnsBadRequest(t *testing.T) {
	original := searchTorrents
	searchTorrents = func(_ context.Context, _ []string, _ string, _ bool) (pipelineResult, error) {
		t.Fatal("searchTorrents should not be called for invalid request")
		return pipelineResult{}, nil
	}
	t.Cleanup(func() {
		searchTorrents = original
	})

	req := httptest.NewRequest(http.MethodGet, "/?Year=2001&isMovie=true", nil)
	rr := httptest.NewRecorder()

	handleRequest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "MovieName is required") {
		t.Fatalf("expected error body to contain %q, got %q", "MovieName is required", rr.Body.String())
	}
}

func TestHandleRequest_NoTorrents_ReturnsNotFound(t *testing.T) {
	t.Setenv("RUTOR_SEARCH_URL", "http://rutor.is/search/0/0/300/0/%s %s (2160p|1080p)")
	t.Setenv("KZ_SEARCH_URL", "http://kinozal.tv/browse.php?s=%s (1080p|2160p)&g=3&c=0&v=0&d=%s&w=0&t=0&f=0")

	original := searchTorrents
	searchTorrents = func(_ context.Context, _ []string, _ string, _ bool) (pipelineResult, error) {
		return pipelineResult{Torrents: []any{}, Count: 0}, nil
	}
	t.Cleanup(func() {
		searchTorrents = original
	})

	req := httptest.NewRequest(http.MethodGet, "/?MovieName=The+Mummy+Returns&Year=2001&isMovie=true", nil)
	rr := httptest.NewRecorder()

	handleRequest(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "No torrents found") {
		t.Fatalf("expected error body to contain %q, got %q", "No torrents found", rr.Body.String())
	}
}

func TestHandleRequest_InvalidIsMovie_ReturnsBadRequest(t *testing.T) {
	original := searchTorrents
	searchTorrents = func(_ context.Context, _ []string, _ string, _ bool) (pipelineResult, error) {
		t.Fatal("searchTorrents should not be called for invalid request")
		return pipelineResult{}, nil
	}
	t.Cleanup(func() {
		searchTorrents = original
	})

	req := httptest.NewRequest(http.MethodGet, "/?MovieName=The+Mummy+Returns&Year=2001&isMovie=abc", nil)
	rr := httptest.NewRecorder()

	handleRequest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid isMovie value") {
		t.Fatalf("expected error body to contain %q, got %q", "invalid isMovie value", rr.Body.String())
	}
}

func TestBuildTrackerURLs_MissingEnv_ReturnsError(t *testing.T) {
	t.Setenv("RUTOR_SEARCH_URL", "")
	t.Setenv("KZ_SEARCH_URL", "")

	_, err := buildTrackerURLs("The Mummy Returns", "2001")
	if err == nil {
		t.Fatal("expected error when URL templates are missing")
	}
}

func TestNormalizeTrackerURL_EncodesPathAndQuery(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "path based URL",
			raw:  "http://rutor.is/search/0/0/300/0/The Mummy Returns 2001 (2160p|1080p)",
			want: "http://rutor.is/search/0/0/300/0/The%20Mummy%20Returns%202001%20%282160p%7C1080p%29",
		},
		{
			name: "query based URL",
			raw:  "http://kinozal.tv/browse.php?s=The Mummy Returns (1080p|2160p)&g=3&c=0&v=0&d=2001&w=0&t=0&f=0",
			want: "http://kinozal.tv/browse.php?c=0&d=2001&f=0&g=3&s=The+Mummy+Returns+%281080p%7C2160p%29&t=0&v=0&w=0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeTrackerURL(tt.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected normalized URL:\nwant: %s\ngot:  %s", tt.want, got)
			}
		})
	}
}

func TestBuildTrackerURLs_NormalizesTemplates(t *testing.T) {
	t.Setenv("RUTOR_SEARCH_URL", "http://rutor.is/search/0/0/300/0/%s %s (2160p|1080p)")
	t.Setenv("KZ_SEARCH_URL", "http://kinozal.tv/browse.php?s=%s (1080p|2160p)&g=3&c=0&v=0&d=%s&w=0&t=0&f=0")
	urls, err := buildTrackerURLs("The Mummy Returns", "2001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d", len(urls))
	}
	if !strings.Contains(urls[0], "The%20Mummy%20Returns") {
		t.Fatalf("expected encoded rutor URL, got %s", urls[0])
	}
	if !strings.Contains(urls[1], "The+Mummy+Returns+") {
		t.Fatalf("expected encoded kinozal URL, got %s", urls[1])
	}
}

func TestGetPort_DefaultAndEnv(t *testing.T) {
	_ = os.Unsetenv("PORT")
	if got := getPort(); got != "8080" {
		t.Fatalf("expected default port 8080, got %s", got)
	}

	t.Setenv("PORT", "9000")
	if got := getPort(); got != "9000" {
		t.Fatalf("expected env port 9000, got %s", got)
	}
}

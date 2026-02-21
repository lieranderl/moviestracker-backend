package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"moviestracker.net/getimdbrating/imdbrating"
)

func TestHandleGetIMDbRating_Success(t *testing.T) {
	original := getIMDbRatingByID
	getIMDbRatingByID = func(imdbID string) ([]byte, error) {
		if imdbID != "tt0468569" {
			t.Fatalf("unexpected imdb id: %s", imdbID)
		}
		return []byte(`{"Id":"tt0468569","Rating":"9.0","Votes":"100"}`), nil
	}
	t.Cleanup(func() {
		getIMDbRatingByID = original
	})

	req := httptest.NewRequest(http.MethodGet, "/?imdb_id=tt0468569", nil)
	rr := httptest.NewRecorder()

	handleGetIMDbRating(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d, body=%q", http.StatusOK, rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}
}

func TestHandleGetIMDbRating_ValidationErrors(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantStatus int
		wantBody   string
	}{
		{name: "missing imdb_id", url: "/", wantStatus: http.StatusBadRequest, wantBody: "imdb_id is required"},
		{name: "invalid format", url: "/?imdb_id=bad-id", wantStatus: http.StatusBadRequest, wantBody: "invalid imdb_id format"},
	}

	original := getIMDbRatingByID
	getIMDbRatingByID = func(_ string) ([]byte, error) {
		t.Fatal("getIMDbRatingByID should not be called for invalid input")
		return nil, nil
	}
	t.Cleanup(func() {
		getIMDbRatingByID = original
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rr := httptest.NewRecorder()

			handleGetIMDbRating(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
			if !strings.Contains(rr.Body.String(), tt.wantBody) {
				t.Fatalf("expected body to contain %q, got %q", tt.wantBody, rr.Body.String())
			}
		})
	}
}

func TestHandleGetIMDbRating_NotFound(t *testing.T) {
	original := getIMDbRatingByID
	getIMDbRatingByID = func(_ string) ([]byte, error) {
		return nil, imdbrating.ErrRatingNotFound
	}
	t.Cleanup(func() {
		getIMDbRatingByID = original
	})

	req := httptest.NewRequest(http.MethodGet, "/?imdb_id=tt0468569", nil)
	rr := httptest.NewRecorder()

	handleGetIMDbRating(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestHandleGetIMDbRating_InternalError(t *testing.T) {
	original := getIMDbRatingByID
	getIMDbRatingByID = func(_ string) ([]byte, error) {
		return nil, errors.New("disk failure")
	}
	t.Cleanup(func() {
		getIMDbRatingByID = original
	})

	req := httptest.NewRequest(http.MethodGet, "/?imdb_id=tt0468569", nil)
	rr := httptest.NewRecorder()

	handleGetIMDbRating(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "failed to load imdb rating") {
		t.Fatalf("unexpected error body: %q", rr.Body.String())
	}
}

//go:build integration

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

func TestHandleRequest_Integration_TheMummyReturns2001(t *testing.T) {
	requireEnvOrSkip(t, "RUTOR_SEARCH_URL")
	requireEnvOrSkip(t, "KZ_SEARCH_URL")
	requireEnvOrSkip(t, "KZ_LOGIN")
	requireEnvOrSkip(t, "KZ_PASSWORD")

	tmdbKey := os.Getenv("TMDBAPIKEY")
	if tmdbKey == "" {
		t.Skip("TMDBAPIKEY is required for integration test")
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/?MovieName=The+Mummy+Returns&Year=2001&isMovie=true&key="+url.QueryEscape(tmdbKey),
		nil,
	)
	rr := httptest.NewRecorder()

	handleRequest(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d, body=%q", http.StatusOK, rr.Code, rr.Body.String())
	}

	var torrents []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &torrents); err != nil {
		t.Fatalf("expected JSON array response, got decode error: %v", err)
	}

	if len(torrents) == 0 {
		t.Fatalf("expected non-empty torrents response, got: %s", rr.Body.String())
	}
}

func requireEnvOrSkip(t *testing.T, name string) {
	t.Helper()
	if os.Getenv(name) == "" {
		t.Skipf("%s is required for integration test", name)
	}
}

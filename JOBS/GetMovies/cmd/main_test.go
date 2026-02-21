package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRun_ValidationErrors(t *testing.T) {
	getenv := func(string) string { return "" }

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing args",
			args:    []string{},
			wantErr: "usage:",
		},
		{
			name:    "invalid db name",
			args:    []string{"unknown", "mongo"},
			wantErr: "unsupported db name",
		},
		{
			name:    "missing TMDB key",
			args:    []string{"latesttorrentsmovies", "mongo"},
			wantErr: "no source URLs configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(context.Background(), tt.args, getenv)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestGetSourceURLs(t *testing.T) {
	getenv := func(key string) string {
		switch key {
		case "RUTOR_URLS":
			return "http://example-1, http://example-2 ,"
		case "RUTOR_HDR10_URLS":
			return "http://hdr10-1"
		case "RUTOR_DV_URLS":
			return "http://dv-1"
		default:
			return ""
		}
	}

	urls, err := getSourceURLs("latesttorrentsmovies", getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 2 || urls[0] != "http://example-1" || urls[1] != "http://example-2" {
		t.Fatalf("unexpected urls: %#v", urls)
	}
}

func TestParseJobTimeout(t *testing.T) {
	if got := parseJobTimeout(""); got != defaultJobTimeout {
		t.Fatalf("expected default timeout, got %s", got)
	}
	if got := parseJobTimeout("30s"); got != 30*time.Second {
		t.Fatalf("expected 30s timeout, got %s", got)
	}
	if got := parseJobTimeout("invalid"); got != defaultJobTimeout {
		t.Fatalf("expected default timeout on invalid input, got %s", got)
	}
}

func TestRun_MissingTMDBKey_WhenURLsExist(t *testing.T) {
	getenv := func(key string) string {
		if key == "RUTOR_URLS" {
			return "http://example-1"
		}
		return ""
	}

	err := run(context.Background(), []string{"latesttorrentsmovies", "mongo"}, getenv)
	if err == nil || !strings.Contains(err.Error(), "TMDBAPIKEY is required") {
		t.Fatalf("expected TMDB key error, got %v", err)
	}
}

func TestRun_FirebaseModeUnsupported(t *testing.T) {
	getenv := func(key string) string {
		switch key {
		case "RUTOR_URLS":
			return "http://example-1"
		case "TMDBAPIKEY":
			return "tmdb-key"
		default:
			return ""
		}
	}

	err := run(context.Background(), []string{"latesttorrentsmovies", "firebase"}, getenv)
	if err == nil || !strings.Contains(err.Error(), "no longer supported") {
		t.Fatalf("expected firebase unsupported error, got %v", err)
	}
}

package imdbrating

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadImdbToFile_Success(t *testing.T) {
	payload := "tconst\taverageRating\tnumVotes\ntt0468569\t9.0\t100\n"
	destPath := filepath.Join(t.TempDir(), "imdb_data.tsv")
	client := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(bytes.NewReader(gzipBytes(t, payload))),
				Header:     make(http.Header),
			}, nil
		}),
	}

	err := DownloadImdbToFile(context.Background(), client, "https://example.test/imdb.gz", destPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed reading destination file: %v", err)
	}
	if string(got) != payload {
		t.Fatalf("unexpected file content:\nwant: %q\ngot:  %q", payload, string(got))
	}
}

func TestDownloadImdbToFile_Non200(t *testing.T) {
	destPath := filepath.Join(t.TempDir(), "imdb_data.tsv")
	client := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Status:     "502 Bad Gateway",
				Body:       io.NopCloser(strings.NewReader("nope")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	err := DownloadImdbToFile(context.Background(), client, "https://example.test/imdb.gz", destPath)
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "unexpected response status") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDownloadImdbToFile_InvalidGzip(t *testing.T) {
	destPath := filepath.Join(t.TempDir(), "imdb_data.tsv")
	client := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("not-gzip")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	err := DownloadImdbToFile(context.Background(), client, "https://example.test/imdb.gz", destPath)
	if err == nil {
		t.Fatal("expected error for invalid gzip payload")
	}
	if !strings.Contains(err.Error(), "create gzip reader") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func gzipBytes(t *testing.T, input string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(input)); err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	return buf.Bytes()
}

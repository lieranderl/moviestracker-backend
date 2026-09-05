package imdb

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDownloadRatingsPreservesValidGzip(t *testing.T) {
	var archive bytes.Buffer
	zipWriter := gzip.NewWriter(&archive)
	_, err := zipWriter.Write([]byte("tt1\t7.0\t10\n"))
	require.NoError(t, err)
	require.NoError(t, zipWriter.Close())

	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(bytes.NewReader(archive.Bytes()))}, nil
	})}

	got, err := DownloadRatings(context.Background(), client, "https://example.test/ratings.gz")

	require.NoError(t, err)
	require.Equal(t, archive.Bytes(), got)
}

func TestDownloadRatingsRejectsInvalidArchive(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader("not gzip"))}, nil
	})}

	_, err := DownloadRatings(context.Background(), client, "https://example.test/ratings.gz")

	require.ErrorContains(t, err, "validate ratings gzip")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

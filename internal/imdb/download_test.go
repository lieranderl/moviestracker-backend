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

func gzipArchive(t *testing.T, raw string) []byte {
	t.Helper()
	var archive bytes.Buffer
	zipWriter := gzip.NewWriter(&archive)
	_, err := zipWriter.Write([]byte(raw))
	require.NoError(t, err)
	require.NoError(t, zipWriter.Close())
	return archive.Bytes()
}

func serving(archive []byte) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(bytes.NewReader(archive))}, nil
	})}
}

func TestDownloadRatingsPreservesValidGzip(t *testing.T) {
	archive := gzipArchive(t, "tconst\taverageRating\tnumVotes\ntt0000001\t7.0\t10\ntt0000002\t8.0\t20\n")

	got, stats, err := DownloadRatings(context.Background(), serving(archive), "https://example.test/ratings.gz", 2)

	require.NoError(t, err)
	require.Equal(t, archive, got)
	require.Equal(t, 2, stats.Rows())
}

// A well-formed gzip whose payload is only the header is exactly the failure the
// gzip check alone let through to Cloud Storage.
func TestDownloadRatingsRejectsArchiveBelowRowFloor(t *testing.T) {
	archive := gzipArchive(t, "tconst\taverageRating\tnumVotes\n")

	got, _, err := DownloadRatings(context.Background(), serving(archive), "https://example.test/ratings.gz", 1)

	require.ErrorIs(t, err, ErrSnapshotTooSmall)
	require.Nil(t, got)
}

func TestDownloadRatingsRejectsTruncatedGzip(t *testing.T) {
	archive := gzipArchive(t, "tt0000001\t7.0\t10\n")

	_, _, err := DownloadRatings(context.Background(), serving(archive[:len(archive)-4]), "https://example.test/ratings.gz", 0)

	require.ErrorContains(t, err, "validate ratings gzip")
}

func TestDownloadRatingsRejectsInvalidArchive(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader("not gzip"))}, nil
	})}

	_, _, err := DownloadRatings(context.Background(), client, "https://example.test/ratings.gz", 0)

	require.ErrorContains(t, err, "validate ratings gzip")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

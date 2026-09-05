package gcsstore

import (
	"bytes"
	"compress/gzip"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeRatingsSnapshotDecompressesGzip(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write([]byte("tt1\t7.0\t10\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	snapshot, err := decodeRatingsSnapshot(42, bytes.NewReader(compressed.Bytes()))

	require.NoError(t, err)
	require.Equal(t, int64(42), snapshot.Generation)
	ratingBytes := new(bytes.Buffer)
	_, err = ratingBytes.ReadFrom(snapshot.Reader)
	require.NoError(t, err)
	require.Equal(t, "tt1\t7.0\t10\n", ratingBytes.String())
}

func TestSourceRejectsMissingConfiguration(t *testing.T) {
	_, err := (RatingsSource{}).Open(context.Background(), 0)
	require.ErrorContains(t, err, "storage client is required")
}

func TestStoreRejectsMissingConfiguration(t *testing.T) {
	err := (RatingsStore{}).Put(context.Background(), []byte("archive"))
	require.ErrorContains(t, err, "storage client is required")
}

package gcsstore

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/require"
)

func TestDecodeRatingsSnapshotStreamsGzip(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write([]byte("tt0000001\t7.0\t10\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	object := &trackedObject{Reader: bytes.NewReader(compressed.Bytes())}

	snapshot, err := decodeRatingsSnapshot(42, object)

	require.NoError(t, err)
	require.Equal(t, int64(42), snapshot.Generation)
	require.False(t, snapshot.Unchanged)
	decoded, err := io.ReadAll(snapshot.Body)
	require.NoError(t, err)
	require.Equal(t, "tt0000001\t7.0\t10\n", string(decoded))

	// Closing the snapshot must also release the underlying object stream.
	require.False(t, object.closed)
	require.NoError(t, snapshot.Body.Close())
	require.True(t, object.closed)
}

func TestDecodeRatingsSnapshotClosesObjectOnBadGzip(t *testing.T) {
	object := &trackedObject{Reader: bytes.NewReader([]byte("not gzip"))}

	_, err := decodeRatingsSnapshot(1, object)

	require.ErrorContains(t, err, "open ratings gzip")
	require.True(t, object.closed, "a failed decode must not leak the object stream")
}

type trackedObject struct {
	*bytes.Reader
	closed bool
}

func (t *trackedObject) Close() error {
	t.closed = true
	return nil
}

func TestPreferIndexOnlyWhenBuiltFromCurrentExport(t *testing.T) {
	export := &storage.ObjectAttrs{Generation: 1761789600000000, Created: time.Date(2026, 10, 30, 2, 0, 0, 0, time.UTC)}
	builtFrom := func(generation string, created time.Time) *storage.ObjectAttrs {
		return &storage.ObjectAttrs{Created: created, Metadata: map[string]string{SourceGenerationKey: generation}}
	}

	require.False(t, preferIndex(export, nil), "missing index")
	require.True(t, preferIndex(export, builtFrom("1761789600000000", export.Created.Add(time.Second))), "built from this export")
	require.False(t, preferIndex(export, builtFrom("1761703200000000", export.Created.Add(time.Minute))),
		"an overlapping older execution can upload its index last; creation order must not win")
	require.False(t, preferIndex(export, &storage.ObjectAttrs{Created: export.Created.Add(time.Second)}), "no source recorded")
}

func TestSourceRejectsMissingConfiguration(t *testing.T) {
	_, err := (RatingsSource{}).Open(context.Background(), 0)
	require.ErrorContains(t, err, "storage client is required")
}

func TestStoreRejectsMissingConfiguration(t *testing.T) {
	_, err := (RatingsStore{}).Put(context.Background(), []byte("archive"))
	require.ErrorContains(t, err, "storage client is required")
}

package gcsstore

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
	"github.com/lieranderl/moviestracker-backend/internal/imdb"
)

type RatingsSource struct {
	Bucket string
	Client *storage.Client
	Object string
}

type RatingsStore struct {
	Bucket string
	Client *storage.Client
	Object string
}

func (s RatingsStore) Put(ctx context.Context, archive []byte) error {
	if s.Client == nil {
		return errors.New("storage client is required")
	}
	if s.Bucket == "" || s.Object == "" {
		return errors.New("storage bucket and object are required")
	}
	object := s.Client.Bucket(s.Bucket).Object(s.Object)
	attrs, err := object.Attrs(ctx)
	if err == nil {
		object = object.If(storage.Conditions{GenerationMatch: attrs.Generation})
	} else if errors.Is(err, storage.ErrObjectNotExist) {
		object = object.If(storage.Conditions{DoesNotExist: true})
	} else {
		return fmt.Errorf("read existing ratings metadata: %w", err)
	}
	writeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	writer := object.NewWriter(writeCtx)
	writer.ContentType = "application/gzip"
	writer.CacheControl = "no-cache"
	if _, err := io.Copy(writer, bytes.NewReader(archive)); err != nil {
		cancel()
		_ = writer.Close()
		return fmt.Errorf("upload ratings archive: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("commit ratings archive: %w", err)
	}
	return nil
}

func (s RatingsSource) Open(ctx context.Context, currentGeneration int64) (imdb.Snapshot, error) {
	if s.Client == nil {
		return imdb.Snapshot{}, errors.New("storage client is required")
	}
	if s.Bucket == "" || s.Object == "" {
		return imdb.Snapshot{}, errors.New("storage bucket and object are required")
	}
	object := s.Client.Bucket(s.Bucket).Object(s.Object)
	attrs, err := object.Attrs(ctx)
	if err != nil {
		return imdb.Snapshot{}, fmt.Errorf("read ratings object metadata: %w", err)
	}
	if currentGeneration != 0 && attrs.Generation == currentGeneration {
		return imdb.Snapshot{Generation: attrs.Generation, Unchanged: true}, nil
	}
	reader, err := object.Generation(attrs.Generation).NewReader(ctx)
	if err != nil {
		return imdb.Snapshot{}, fmt.Errorf("open ratings object: %w", err)
	}
	return decodeRatingsSnapshot(attrs.Generation, reader)
}

// decodeRatingsSnapshot wraps the object stream in a gzip decoder and hands both
// to the caller to close. The whole archive is deliberately not buffered: the
// decompressed export is around 30 MiB and the parser consumes it as a stream.
func decodeRatingsSnapshot(generation int64, reader io.ReadCloser) (imdb.Snapshot, error) {
	archive, err := gzip.NewReader(reader)
	if err != nil {
		_ = reader.Close()
		return imdb.Snapshot{}, fmt.Errorf("open ratings gzip: %w", err)
	}
	return imdb.Snapshot{Generation: generation, Body: gzipReadCloser{Reader: archive, object: reader}}, nil
}

// gzipReadCloser closes the gzip decoder and the underlying object stream so a
// refresh cannot leak an open GCS connection.
type gzipReadCloser struct {
	*gzip.Reader
	object io.Closer
}

func (g gzipReadCloser) Close() error {
	err := g.Reader.Close()
	if objectErr := g.object.Close(); err == nil {
		err = objectErr
	}
	return err
}

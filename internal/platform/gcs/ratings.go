package gcsstore

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
	"github.com/lieranderl/moviestracker-backend/internal/imdb"
)

// RatingsSource reads the published ratings. When IndexObject is set and holds
// an index at least as new as the export in Object, the index is preferred.
type RatingsSource struct {
	Bucket      string
	Client      *storage.Client
	IndexObject string
	Object      string
}

type RatingsStore struct {
	Bucket string
	Client *storage.Client
	// ContentType defaults to application/gzip, the export's type.
	ContentType string
	Object      string
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
	writer.ContentType = cmp.Or(s.ContentType, "application/gzip")
	writer.CacheControl = "no-cache"
	if _, err := io.Copy(writer, bytes.NewReader(archive)); err != nil {
		cancel()
		_ = writer.Close()
		return fmt.Errorf("upload %s: %w", s.Object, err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("commit %s: %w", s.Object, err)
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
	bucket := s.Client.Bucket(s.Bucket)
	exportAttrs, err := bucket.Object(s.Object).Attrs(ctx)
	if err != nil {
		return imdb.Snapshot{}, fmt.Errorf("read ratings object metadata: %w", err)
	}
	var indexAttrs *storage.ObjectAttrs
	if s.IndexObject != "" {
		indexAttrs, err = bucket.Object(s.IndexObject).Attrs(ctx)
		if errors.Is(err, storage.ErrObjectNotExist) {
			indexAttrs = nil
		} else if err != nil {
			return imdb.Snapshot{}, fmt.Errorf("read ratings index metadata: %w", err)
		}
	}
	if !preferIndex(exportAttrs, indexAttrs) {
		return openSnapshot(ctx, bucket.Object(s.Object), exportAttrs.Generation, currentGeneration, imdb.FormatExport)
	}
	return openSnapshot(ctx, bucket.Object(s.IndexObject), indexAttrs.Generation, currentGeneration, imdb.FormatIndex)
}

// preferIndex reports whether the binary index reflects the current export.
// The download job writes the export first and the index from the same parse
// second, so a current index is never older than the export. An older one was
// left behind by a job that no longer writes it, and serving it would freeze
// the ratings; the export is used instead.
func preferIndex(export, index *storage.ObjectAttrs) bool {
	return index != nil && !index.Created.Before(export.Created)
}

func openSnapshot(ctx context.Context, object *storage.ObjectHandle, generation, currentGeneration int64, format imdb.SnapshotFormat) (imdb.Snapshot, error) {
	if currentGeneration != 0 && generation == currentGeneration {
		return imdb.Snapshot{Generation: generation, Format: format, Unchanged: true}, nil
	}
	reader, err := object.Generation(generation).NewReader(ctx)
	if err != nil {
		return imdb.Snapshot{}, fmt.Errorf("open ratings object %s: %w", object.ObjectName(), err)
	}
	if format == imdb.FormatIndex {
		return imdb.Snapshot{Generation: generation, Format: format, Body: reader}, nil
	}
	return decodeRatingsSnapshot(generation, reader)
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

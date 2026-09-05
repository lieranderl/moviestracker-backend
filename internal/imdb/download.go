package imdb

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
)

const maxRatingsArchiveSize = 64 << 20

func DownloadRatings(ctx context.Context, client *http.Client, sourceURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build ratings request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download ratings: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download ratings: unexpected status %s", response.Status)
	}
	archive, err := io.ReadAll(io.LimitReader(response.Body, maxRatingsArchiveSize+1))
	if err != nil {
		return nil, fmt.Errorf("read ratings archive: %w", err)
	}
	if len(archive) > maxRatingsArchiveSize {
		return nil, fmt.Errorf("ratings archive exceeds %d bytes", maxRatingsArchiveSize)
	}
	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("validate ratings gzip: %w", err)
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		_ = reader.Close()
		return nil, fmt.Errorf("validate ratings gzip: %w", err)
	}
	if err := reader.Close(); err != nil {
		return nil, fmt.Errorf("validate ratings gzip: %w", err)
	}
	return archive, nil
}

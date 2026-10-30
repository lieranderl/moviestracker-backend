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

// DownloadRatings fetches the ratings archive and parses it end to end before
// returning it, so that only an archive the service would accept is published.
func DownloadRatings(ctx context.Context, client *http.Client, sourceURL string, minRows int) ([]byte, ParseStats, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, ParseStats{}, fmt.Errorf("build ratings request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, ParseStats{}, fmt.Errorf("download ratings: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, ParseStats{}, fmt.Errorf("download ratings: unexpected status %s", response.Status)
	}
	archive, err := io.ReadAll(io.LimitReader(response.Body, maxRatingsArchiveSize+1))
	if err != nil {
		return nil, ParseStats{}, fmt.Errorf("read ratings archive: %w", err)
	}
	if len(archive) > maxRatingsArchiveSize {
		return nil, ParseStats{}, fmt.Errorf("ratings archive exceeds %d bytes", maxRatingsArchiveSize)
	}
	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, ParseStats{}, fmt.Errorf("validate ratings gzip: %w", err)
	}
	// Parsing to EOF also verifies the gzip checksum.
	_, stats, err := ParseRatingsWithStats(reader)
	if err != nil {
		_ = reader.Close()
		return nil, stats, fmt.Errorf("validate ratings gzip: %w", err)
	}
	if err := reader.Close(); err != nil {
		return nil, stats, fmt.Errorf("validate ratings gzip: %w", err)
	}
	if err := CheckMinRows(stats, minRows); err != nil {
		return nil, stats, err
	}
	return archive, stats, nil
}

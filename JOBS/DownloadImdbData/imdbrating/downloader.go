package imdbrating

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	urlBase         = "https://datasets.imdbws.com/"
	ratingsFileArch = "title.ratings.tsv.gz"
	defaultDestPath = "./store/imdb_data.tsv"
)

func DownloadImdb() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	client := &http.Client{Timeout: 2 * time.Minute}
	return DownloadImdbToFile(ctx, client, urlBase+ratingsFileArch, defaultDestPath)
}

func DownloadImdbToFile(ctx context.Context, client *http.Client, sourceURL, destPath string) error {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download imdb ratings archive: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response status: %s", resp.Status)
	}

	archive, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer archive.Close()

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	tmpPath := destPath + ".tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	defer func() {
		_ = tmpFile.Close()
	}()

	written, err := io.Copy(tmpFile, archive)
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("decompress imdb data: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temporary file: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace destination file: %w", err)
	}

	elapsed := time.Since(start)
	log.Printf("downloaded %d bytes to %s in %s", written, destPath, elapsed)
	return nil
}

package imdbrating

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGetImdbRatingByIdFromFile_ReturnsExactMatch(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "imdb_data.tsv")
	content := "tt0000001\t5.6\t100\n" +
		"tt1234567\t8.0\t555\n" +
		"tt12345678\t9.0\t999\n"
	if err := os.WriteFile(dataPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write test data: %v", err)
	}

	got, err := GetImdbRatingByIdFromFile(dataPath, "tt1234567")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out IMDbRating
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	if out.ID != "tt1234567" || out.Rating != "8.0" || out.Votes != "555" {
		t.Fatalf("unexpected parsed output: %#v", out)
	}
}

func TestGetImdbRatingByIdFromFile_NotFound(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "imdb_data.tsv")
	if err := os.WriteFile(dataPath, []byte("tt0000001\t5.6\t100\n"), 0o644); err != nil {
		t.Fatalf("write test data: %v", err)
	}

	_, err := GetImdbRatingByIdFromFile(dataPath, "tt9999999")
	if !errors.Is(err, ErrRatingNotFound) {
		t.Fatalf("expected ErrRatingNotFound, got %v", err)
	}
}

func TestGetImdbRatingByIdFromFile_EmptyID(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "imdb_data.tsv")
	if err := os.WriteFile(dataPath, []byte("tt0000001\t5.6\t100\n"), 0o644); err != nil {
		t.Fatalf("write test data: %v", err)
	}

	_, err := GetImdbRatingByIdFromFile(dataPath, "")
	if err == nil {
		t.Fatal("expected error for empty imdb id")
	}
}

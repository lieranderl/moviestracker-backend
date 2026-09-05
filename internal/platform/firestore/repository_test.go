package firestorestore

import (
	"context"
	"testing"
	"time"

	"github.com/lieranderl/moviestracker-backend/internal/catalog"
	"github.com/stretchr/testify/require"
)

func TestUpsertRejectsMovieWithoutStableDocumentID(t *testing.T) {
	repository := Repository{}

	err := repository.Upsert(context.Background(), catalog.CollectionLatest, []catalog.Movie{{Title: "Missing ID"}})

	require.ErrorContains(t, err, "movie id is required")
}

func TestDocumentDataUsesNativeTimestamp(t *testing.T) {
	found := time.Date(2026, 9, 5, 1, 2, 3, 0, time.UTC)

	data := documentData(catalog.Movie{ID: "9737", LastTimeFound: found, Title: "Bad Boys"})

	require.Equal(t, found, data["lasttimefound"])
	require.Equal(t, "9737", data["id"])
}

func TestDeduplicateMoviesKeepsNewestTMDBDocument(t *testing.T) {
	old := catalog.Movie{ID: "1", LastTimeFound: time.Unix(1, 0), Title: "old"}
	newest := catalog.Movie{ID: "1", LastTimeFound: time.Unix(2, 0), Title: "new"}

	got := deduplicateMovies([]catalog.Movie{old, newest})

	require.Equal(t, []catalog.Movie{newest}, got)
}

package firestorestore

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/lieranderl/moviestracker-backend/internal/catalog"
	"golang.org/x/sync/errgroup"
)

type Repository struct {
	Client *firestore.Client
}

func New(client *firestore.Client) *Repository { return &Repository{Client: client} }

// readBatchSize bounds one BatchGetDocuments call. Firestore accepts more, but
// a few hundred catalog documents need only a handful of round trips.
const readBatchSize = 100

// Upsert stores movies and reports how many documents it actually wrote.
//
// Most of a scheduled run re-observes movies that are already stored with
// identical data, and every write is billed along with its index entries. The
// existing documents are therefore read in batches first, and only the movies
// whose stored document would change go through the write transaction.
func (r Repository) Upsert(ctx context.Context, collection catalog.Collection, movies []catalog.Movie) (int, error) {
	if !collection.Valid() {
		return 0, fmt.Errorf("unsupported collection %q", collection)
	}
	for _, movie := range movies {
		if movie.ID == "" {
			return 0, errors.New("movie id is required for Firestore document")
		}
	}
	movies = deduplicateMovies(movies)
	if len(movies) == 0 {
		return 0, nil
	}
	if r.Client == nil {
		return 0, errors.New("firestore client is required")
	}

	changed, err := r.changedMovies(ctx, collection, movies)
	if err != nil {
		return 0, err
	}

	written := make([]bool, len(changed))
	var writes errgroup.Group
	writes.SetLimit(20)
	for index, movie := range changed {
		writes.Go(func() error {
			var err error
			written[index], err = r.upsertMovie(ctx, collection, movie)
			return err
		})
	}
	if err := writes.Wait(); err != nil {
		return 0, err
	}
	count := 0
	for _, wrote := range written {
		if wrote {
			count++
		}
	}
	return count, nil
}

// changedMovies returns the movies whose stored document is missing or differs
// from what upsertMovie would write. The read is outside any transaction, so it
// only filters; upsertMovie re-reads and decides again under the transaction.
func (r Repository) changedMovies(ctx context.Context, collection catalog.Collection, movies []catalog.Movie) ([]catalog.Movie, error) {
	changed := make([]catalog.Movie, 0, len(movies))
	for batch := range slices.Chunk(movies, readBatchSize) {
		references := make([]*firestore.DocumentRef, len(batch))
		for index, movie := range batch {
			references[index] = r.Client.Collection(string(collection)).Doc(movie.ID)
		}
		snapshots, err := r.Client.GetAll(ctx, references)
		if err != nil {
			return nil, fmt.Errorf("read Firestore movies: %w", err)
		}
		for index, movie := range batch {
			if _, unchanged := mergedDocument(snapshots[index], movie); !unchanged {
				changed = append(changed, movie)
			}
		}
	}
	return changed, nil
}

func (r Repository) upsertMovie(ctx context.Context, collection catalog.Collection, movie catalog.Movie) (bool, error) {
	reference := r.Client.Collection(string(collection)).Doc(movie.ID)
	var wrote bool
	// A timestamp cannot use Firestore's numeric Maximum transform. Read and
	// replace atomically so overlapping jobs and SDK conflict retries retain the
	// newest observed discovery time while still updating the remaining metadata.
	err := r.Client.RunTransaction(ctx, func(_ context.Context, transaction *firestore.Transaction) error {
		wrote = false
		snapshots, err := transaction.GetAll([]*firestore.DocumentRef{reference})
		if err != nil {
			return err
		}
		data, unchanged := mergedDocument(snapshots[0], movie)
		if unchanged {
			return nil
		}
		wrote = true
		return transaction.Set(reference, data)
	})
	if err != nil {
		return false, fmt.Errorf("commit Firestore movie %s: %w", movie.ID, err)
	}
	return wrote, nil
}

// mergedDocument returns the document to store for movie, keeping a newer
// stored discovery time, and whether the stored document already equals it.
func mergedDocument(snapshot *firestore.DocumentSnapshot, movie catalog.Movie) (map[string]any, bool) {
	data := documentData(movie)
	if snapshot == nil || !snapshot.Exists() {
		return data, false
	}
	existing := snapshot.Data()
	if previous, ok := existing["lasttimefound"].(time.Time); ok && previous.After(movie.LastTimeFound) {
		data["lasttimefound"] = previous
	}
	return data, sameDocument(existing, data)
}

// sameDocument compares a stored document with the data Set would write. It is
// exact about shape: a missing, extra or null-versus-empty field counts as a
// change, so skipping a write never leaves a document that Set would not have.
func sameDocument(existing, data map[string]any) bool {
	if len(existing) != len(data) {
		return false
	}
	for key, want := range data {
		got, ok := existing[key]
		if !ok || !sameValue(got, want) {
			return false
		}
	}
	return true
}

// sameValue compares one field as the SDK reads it back: timestamps as
// time.Time, and integer arrays as []any of int64, or nil for a nil slice.
func sameValue(got, want any) bool {
	switch want := want.(type) {
	case time.Time:
		stored, ok := got.(time.Time)
		return ok && stored.Equal(want)
	case []int32:
		if want == nil || got == nil {
			return want == nil && got == nil
		}
		stored, ok := got.([]any)
		if !ok || len(stored) != len(want) {
			return false
		}
		for index, value := range want {
			if number, ok := stored[index].(int64); !ok || number != int64(value) {
				return false
			}
		}
		return true
	case string:
		stored, ok := got.(string)
		return ok && stored == want
	default:
		return false
	}
}

func deduplicateMovies(movies []catalog.Movie) []catalog.Movie {
	byID := make(map[string]catalog.Movie, len(movies))
	order := make([]string, 0, len(movies))
	for _, movie := range movies {
		current, exists := byID[movie.ID]
		if !exists {
			order = append(order, movie.ID)
		}
		if !exists || movie.LastTimeFound.After(current.LastTimeFound) {
			byID[movie.ID] = movie
		}
	}
	result := make([]catalog.Movie, 0, len(byID))
	for _, id := range order {
		result = append(result, byID[id])
	}
	return result
}

func documentData(movie catalog.Movie) map[string]any {
	return map[string]any{
		"backdrop_path":  movie.BackdropPath,
		"genre_ids":      movie.GenreIDs,
		"id":             movie.ID,
		"lasttimefound":  movie.LastTimeFound,
		"original_title": movie.OriginalTitle,
		"poster_path":    movie.PosterPath,
		"release_date":   movie.ReleaseDate,
		"title":          movie.Title,
		"vote_average":   movie.VoteAverage,
		"vote_count":     movie.VoteCount,
		"Year":           movie.Year,
	}
}

package firestorestore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/lieranderl/moviestracker-backend/internal/catalog"
	"golang.org/x/sync/errgroup"
)

type Repository struct {
	Client *firestore.Client
}

func New(client *firestore.Client) *Repository { return &Repository{Client: client} }

func (r Repository) Upsert(ctx context.Context, collection catalog.Collection, movies []catalog.Movie) error {
	if !collection.Valid() {
		return fmt.Errorf("unsupported collection %q", collection)
	}
	for _, movie := range movies {
		if movie.ID == "" {
			return errors.New("movie id is required for Firestore document")
		}
	}
	movies = deduplicateMovies(movies)
	if len(movies) == 0 {
		return nil
	}
	if r.Client == nil {
		return errors.New("firestore client is required")
	}

	var writes errgroup.Group
	writes.SetLimit(20)
	for _, movie := range movies {
		writes.Go(func() error {
			return r.upsertMovie(ctx, collection, movie)
		})
	}
	return writes.Wait()
}

func (r Repository) upsertMovie(ctx context.Context, collection catalog.Collection, movie catalog.Movie) error {
	reference := r.Client.Collection(string(collection)).Doc(movie.ID)
	// A timestamp cannot use Firestore's numeric Maximum transform. Read and
	// replace atomically so overlapping jobs and SDK conflict retries retain the
	// newest observed discovery time while still updating the remaining metadata.
	err := r.Client.RunTransaction(ctx, func(_ context.Context, transaction *firestore.Transaction) error {
		snapshots, err := transaction.GetAll([]*firestore.DocumentRef{reference})
		if err != nil {
			return err
		}
		data := documentData(movie)
		if snapshots[0].Exists() {
			if previous, ok := snapshots[0].Data()["lasttimefound"].(time.Time); ok && previous.After(movie.LastTimeFound) {
				data["lasttimefound"] = previous
			}
		}
		return transaction.Set(reference, data)
	})
	if err != nil {
		return fmt.Errorf("commit Firestore movie %s: %w", movie.ID, err)
	}
	return nil
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

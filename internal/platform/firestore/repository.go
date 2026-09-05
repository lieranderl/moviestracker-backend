package firestorestore

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/lieranderl/moviestracker-backend/internal/catalog"
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

	writer := r.Client.BulkWriter(ctx)
	jobs := make([]*firestore.BulkWriterJob, 0, len(movies))
	for _, movie := range movies {
		job, err := writer.Set(r.Client.Collection(string(collection)).Doc(movie.ID), documentData(movie))
		if err != nil {
			writer.End()
			return fmt.Errorf("queue Firestore movie: %w", err)
		}
		jobs = append(jobs, job)
	}
	writer.End()
	for _, job := range jobs {
		if _, err := job.Results(); err != nil {
			return fmt.Errorf("commit Firestore movie: %w", err)
		}
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

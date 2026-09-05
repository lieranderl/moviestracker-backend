package catalog

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"
)

type Tracker interface {
	Fetch(context.Context, string, bool) ([]Torrent, error)
}

type TrackerFunc func(context.Context, string, bool) ([]Torrent, error)

func (f TrackerFunc) Fetch(ctx context.Context, url string, series bool) ([]Torrent, error) {
	return f(ctx, url, series)
}

type Enricher interface {
	Enrich(context.Context, Movie) (Movie, error)
}

type MovieRepository interface {
	Upsert(context.Context, Collection, []Movie) error
}

type RunConfig struct {
	Collection Collection
	URLs       []string
}

type Runner struct {
	Enricher   Enricher
	Repository MovieRepository
	Tracker    Tracker
}

func (r Runner) Run(ctx context.Context, config RunConfig) error {
	if r.Tracker == nil {
		return errors.New("tracker is required")
	}
	if !config.Collection.Valid() {
		return fmt.Errorf("unsupported collection %q", config.Collection)
	}
	if len(config.URLs) == 0 {
		return errors.New("at least one tracker URL is required")
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(3)
	var mu sync.Mutex
	all := make([]Torrent, 0)
	for _, trackerURL := range config.URLs {
		trackerURL := trackerURL
		group.Go(func() error {
			torrents, err := r.Tracker.Fetch(groupCtx, trackerURL, false)
			if err != nil {
				return err
			}
			mu.Lock()
			all = append(all, torrents...)
			mu.Unlock()
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}

	movies := GroupMovies(all)
	if r.Enricher != nil {
		enriched := make([]Movie, len(movies))
		enrichGroup, enrichCtx := errgroup.WithContext(ctx)
		enrichGroup.SetLimit(20)
		for index := range movies {
			index := index
			enrichGroup.Go(func() error {
				movie, err := r.Enricher.Enrich(enrichCtx, movies[index])
				if err != nil {
					return err
				}
				enriched[index] = movie
				return nil
			})
		}
		if err := enrichGroup.Wait(); err != nil {
			return err
		}
		movies = enriched
	}

	filtered := movies[:0]
	for _, movie := range movies {
		if movie.ID != "" || r.Enricher == nil {
			filtered = append(filtered, movie)
		}
	}
	if r.Repository != nil {
		return r.Repository.Upsert(ctx, config.Collection, filtered)
	}
	return nil
}

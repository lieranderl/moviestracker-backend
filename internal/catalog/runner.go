package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"golang.org/x/sync/errgroup"
)

// maxFailureRatio is the share of per-item failures tolerated before a run is
// treated as broken rather than merely degraded.
//
// The distinction matters because the two failure modes look different: a
// rejected API key, an expired tracker domain or a TMDB outage fails nearly
// every item, while a transient timeout fails a handful. Losing a few titles
// until the next scheduled run is far better than discarding a whole scrape,
// but silently writing a near-empty catalog would be worse than failing loudly.
const maxFailureRatio = 0.5

const (
	trackerConcurrency         = 3
	defaultEnricherConcurrency = 10
	maxEnricherConcurrency     = 20
)

type Tracker interface {
	Fetch(context.Context, string, bool) ([]Torrent, error)
}

type TrackerBatch struct {
	Fetched         int
	RejectedParsing int
	Torrents        []Torrent
}

type DiagnosticTracker interface {
	FetchBatch(context.Context, string, bool) (TrackerBatch, error)
}

type TrackerFunc func(context.Context, string, bool) ([]Torrent, error)

func (f TrackerFunc) Fetch(ctx context.Context, url string, series bool) ([]Torrent, error) {
	return f(ctx, url, series)
}

type Enricher interface {
	Enrich(context.Context, Movie) (Movie, error)
}

// MovieRepository stores resolved movies and reports how many documents it
// actually wrote, which is fewer than it was given when stored data is current.
type MovieRepository interface {
	Upsert(context.Context, Collection, []Movie) (int, error)
}

type RunConfig struct {
	Collection          Collection
	EnricherConcurrency int
	URLs                []string
}

// Result reports what a run actually achieved, so the caller can log the counts
// and a degraded run is visible rather than indistinguishable from a clean one.
type Result struct {
	Failed          int
	Fetched         int
	Grouped         int
	RejectedParsing int
	Resolved        int
	Stored          int
	Unresolved      int
	Written         int
}

// ResolvedRatio is the share of grouped movies that matched a TMDB entry. A run
// that grouped nothing reports zero: it stored nothing, which deserves the same
// attention as a collapse in matching.
//
// Unresolved movies are not failures, so maxFailureRatio never sees them; this
// ratio is what the catalog_low_resolution alert in infra/prod/monitoring.tf
// watches to catch a matching regression or a changed TMDB response.
func (r Result) ResolvedRatio() float64 {
	if r.Grouped == 0 {
		return 0
	}
	return float64(r.Resolved) / float64(r.Grouped)
}

type Runner struct {
	Enricher   Enricher
	Repository MovieRepository
	Tracker    Tracker
}

func (r Runner) Run(ctx context.Context, config RunConfig) (Result, error) {
	var result Result
	if r.Tracker == nil {
		return result, errors.New("tracker is required")
	}
	if !config.Collection.Valid() {
		return result, fmt.Errorf("unsupported collection %q", config.Collection)
	}
	if len(config.URLs) == 0 {
		return result, errors.New("at least one tracker URL is required")
	}
	concurrency, err := validatedEnricherConcurrency(config.EnricherConcurrency)
	if err != nil {
		return result, err
	}

	torrents, err := r.fetchAll(ctx, config, &result)
	if err != nil {
		return result, err
	}

	movies := GroupMovies(torrents)
	result.Grouped = len(movies)

	movies, err = r.enrichAll(ctx, movies, concurrency, &result)
	if err != nil {
		return result, err
	}

	if r.Repository != nil {
		written, err := r.Repository.Upsert(ctx, config.Collection, movies)
		if err != nil {
			return result, err
		}
		result.Written = written
	}
	result.Stored = len(movies)
	return result, nil
}

// fetchAll scrapes every configured page concurrently. A page that fails is
// reported and skipped; only losing every page fails the run.
func (r Runner) fetchAll(ctx context.Context, config RunConfig, result *Result) ([]Torrent, error) {
	var mu sync.Mutex
	all := make([]Torrent, 0)
	failures := make([]error, 0)

	// Deliberately not errgroup.WithContext: cancelling siblings on the first
	// error is what previously turned one bad page into a lost run.
	var group errgroup.Group
	group.SetLimit(trackerConcurrency)
	for _, trackerURL := range config.URLs {
		group.Go(func() error {
			batch := TrackerBatch{}
			var err error
			if tracker, ok := r.Tracker.(DiagnosticTracker); ok {
				batch, err = tracker.FetchBatch(ctx, trackerURL, false)
			} else {
				batch.Torrents, err = r.Tracker.Fetch(ctx, trackerURL, false)
				batch.Fetched = len(batch.Torrents)
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures = append(failures, fmt.Errorf("fetch %s: %w", trackerURL, err))
				return nil
			}
			all = append(all, batch.Torrents...)
			result.Fetched += batch.Fetched
			result.RejectedParsing += batch.RejectedParsing
			return nil
		})
	}
	_ = group.Wait()

	result.Failed += len(failures)
	if len(failures) == len(config.URLs) {
		return nil, fmt.Errorf("every tracker source failed: %w", errors.Join(failures...))
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	for _, err := range failures {
		slog.Warn("tracker source failed; continuing with remaining sources", "error", err)
	}
	return all, nil
}

// enrichAll resolves every grouped movie against the enricher and keeps the ones
// that resolved. Individual failures are tolerated up to maxFailureRatio.
func (r Runner) enrichAll(ctx context.Context, movies []Movie, concurrency int, result *Result) ([]Movie, error) {
	if r.Enricher == nil {
		result.Resolved = len(movies)
		return movies, nil
	}

	enriched := make([]Movie, len(movies))
	failures := make([]error, len(movies))

	var group errgroup.Group
	group.SetLimit(concurrency)
	for index := range movies {
		group.Go(func() error {
			movie, err := r.Enricher.Enrich(ctx, movies[index])
			if err != nil {
				failures[index] = fmt.Errorf("enrich %q (%s): %w", movies[index].SearchName, movies[index].Year, err)
				return nil
			}
			enriched[index] = movie
			return nil
		})
	}
	_ = group.Wait()

	resolved := make([]Movie, 0, len(movies))
	failed := make([]error, 0)
	for index, movie := range enriched {
		switch {
		case failures[index] != nil:
			failed = append(failed, failures[index])
		case movie.ID != "":
			resolved = append(resolved, movie)
		}
	}

	result.Failed += len(failed)
	result.Resolved = len(resolved)
	result.Unresolved = len(movies) - len(resolved) - len(failed)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if len(movies) > 0 && float64(len(failed)) > maxFailureRatio*float64(len(movies)) {
		return nil, fmt.Errorf("enrichment failed for %d of %d movies: %w", len(failed), len(movies), errors.Join(failed...))
	}
	for _, err := range failed {
		slog.Warn("enrichment failed; movie skipped for this run", "error", err)
	}
	return resolved, nil
}

func validatedEnricherConcurrency(value int) (int, error) {
	if value == 0 {
		return defaultEnricherConcurrency, nil
	}
	if value < 1 || value > maxEnricherConcurrency {
		return 0, fmt.Errorf("enricher concurrency must be between 1 and %d", maxEnricherConcurrency)
	}
	return value, nil
}

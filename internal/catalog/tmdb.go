package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultTMDBBaseURL = "https://api.themoviedb.org/3"

type TMDBClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewTMDBClient(client *http.Client, baseURL, apiKey string) *TMDBClient {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultTMDBBaseURL
	}
	return &TMDBClient{apiKey: strings.TrimSpace(apiKey), baseURL: strings.TrimRight(baseURL, "/"), client: client}
}

func (c *TMDBClient) Enrich(ctx context.Context, movie Movie) (Movie, error) {
	endpoint, err := url.Parse(c.baseURL + "/search/movie")
	if err != nil {
		return movie, fmt.Errorf("parse TMDB endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("api_key", c.apiKey)
	query.Set("language", "ru")
	query.Set("query", movie.SearchName)
	if movie.Year != "" {
		query.Set("year", movie.Year)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return movie, fmt.Errorf("build TMDB request: %w", err)
	}
	resp, err := c.doWithRetry(ctx, req)
	if err != nil {
		return movie, fmt.Errorf("search TMDB: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return movie, fmt.Errorf("search TMDB: unexpected status %s", resp.Status)
	}

	var payload struct {
		Results []struct {
			BackdropPath  string  `json:"backdrop_path"`
			GenreIDs      []int32 `json:"genre_ids"`
			ID            int64   `json:"id"`
			OriginalTitle string  `json:"original_title"`
			PosterPath    string  `json:"poster_path"`
			ReleaseDate   string  `json:"release_date"`
			Title         string  `json:"title"`
			VoteAverage   float64 `json:"vote_average"`
			VoteCount     int64   `json:"vote_count"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return movie, fmt.Errorf("decode TMDB response: %w", err)
	}
	if len(payload.Results) == 0 {
		return movie, nil
	}

	// Every result is considered, not just the first. TMDB routinely ranks a
	// same-titled release from an adjacent year above the intended one.
	candidates := make([]Candidate, 0, len(payload.Results))
	for index, result := range payload.Results {
		candidates = append(candidates, Candidate{
			OriginalTitle: result.OriginalTitle,
			Rank:          index,
			ReleaseYear:   releaseYear(result.ReleaseDate),
			Title:         result.Title,
			VoteCount:     result.VoteCount,
		})
	}
	selected := SelectCandidate(movie.SearchName, movie.Year, candidates)
	if selected < 0 {
		return movie, nil
	}
	result := payload.Results[selected]

	movie.BackdropPath = result.BackdropPath
	movie.GenreIDs = result.GenreIDs
	movie.ID = strconv.FormatInt(result.ID, 10)
	movie.OriginalTitle = result.OriginalTitle
	movie.PosterPath = result.PosterPath
	movie.ReleaseDate = result.ReleaseDate
	movie.Title = result.Title
	movie.VoteAverage = strconv.FormatFloat(result.VoteAverage, 'f', 1, 64)
	movie.VoteCount = strconv.FormatInt(result.VoteCount, 10)
	return movie, nil
}

func releaseYear(releaseDate string) string {
	if len(releaseDate) < 4 {
		return ""
	}
	return releaseDate[:4]
}

func (c *TMDBClient) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		resp, err := c.client.Do(req.Clone(ctx))
		if err == nil && !retryableTMDBStatus(resp.StatusCode) {
			return resp, nil
		}
		if ctx.Err() != nil {
			if resp != nil {
				_ = resp.Body.Close()
			}
			return nil, ctx.Err()
		}
		if attempt == maxAttempts-1 {
			if err != nil {
				return nil, err
			}
			return resp, nil
		}

		delay := time.Duration(1<<attempt) * 250 * time.Millisecond
		if resp != nil {
			delay = tmdbRetryDelay(resp, delay)
			_ = resp.Body.Close()
		}
		if err := waitForTMDBRetry(ctx, delay); err != nil {
			return nil, err
		}
	}
	return nil, errors.New("TMDB request exhausted retries")
}

func retryableTMDBStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func tmdbRetryDelay(resp *http.Response, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		if delay := time.Until(at); delay > 0 {
			return delay
		}
		return 0
	}
	return fallback
}

func waitForTMDBRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

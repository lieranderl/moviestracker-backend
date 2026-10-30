package catalog

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTMDBClientEnrichesMatchingMovie(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/search/movie", r.URL.Path)
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		require.False(t, r.URL.Query().Has("api_key"))
		require.Equal(t, "Bad Boys", r.URL.Query().Get("query"))
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`{"results":[{"id":9737,"title":"Плохие парни","original_title":"Bad Boys","release_date":"1995-04-07","backdrop_path":"/b.jpg","poster_path":"/p.jpg","genre_ids":[28,35],"vote_average":6.8,"vote_count":6200}]}`))}, nil
	})}

	tmdb := NewTMDBClient(client, "https://tmdb.test", " secret ")
	got, err := tmdb.Enrich(context.Background(), Movie{SearchName: "Bad Boys", Year: "1995"})

	require.NoError(t, err)
	require.Equal(t, "9737", got.ID)
	require.Equal(t, "Bad Boys", got.OriginalTitle)
	require.Equal(t, "6.8", got.VoteAverage)
}

func TestTMDBClientRejectsOversizedResponseAfterValidJSON(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"results":[]}` + strings.Repeat(" ", 1<<20))),
		}, nil
	})}

	_, err := NewTMDBClient(client, "https://tmdb.test", "secret").Enrich(context.Background(), Movie{SearchName: "Movie"})

	require.ErrorContains(t, err, "response exceeds")
}

func TestTMDBClientAcceptsResponseAtSizeLimit(t *testing.T) {
	json := `{"results":[]}`
	body := json + strings.Repeat(" ", (1<<20)-len(json))
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}

	_, err := NewTMDBClient(client, "https://tmdb.test", "secret").Enrich(context.Background(), Movie{SearchName: "Movie"})

	require.NoError(t, err)
}

func TestTMDBClientRejectsOversizedJSONValue(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"results":[],"padding":"` + strings.Repeat("a", 1<<20) + `"}`)),
		}, nil
	})}

	_, err := NewTMDBClient(client, "https://tmdb.test", "secret").Enrich(context.Background(), Movie{SearchName: "Movie"})

	require.ErrorContains(t, err, "response exceeds")
}

func TestTMDBClientRetriesRateLimit(t *testing.T) {
	var attempts atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		require.False(t, r.URL.Query().Has("api_key"))
		if attempts.Add(1) == 1 {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Status:     "429 Too Many Requests",
				Header:     http.Header{"Retry-After": []string{"0"}},
				Body:       io.NopCloser(strings.NewReader("rate limited")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"results":[]}`)),
		}, nil
	})}

	_, err := NewTMDBClient(client, "https://tmdb.test", "secret").Enrich(context.Background(), Movie{SearchName: "Bad Boys"})

	require.NoError(t, err)
	require.EqualValues(t, 2, attempts.Load())
}

func TestTMDBClientCancelsRateLimitWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		cancel()
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Status:     "429 Too Many Requests",
			Header:     http.Header{"Retry-After": []string{"60"}},
			Body:       io.NopCloser(strings.NewReader("rate limited")),
		}, nil
	})}

	_, err := NewTMDBClient(client, "https://tmdb.test", "secret").Enrich(ctx, Movie{SearchName: "Bad Boys"})

	require.ErrorIs(t, err, context.Canceled)
}

func TestTMDBClientLeavesNonMatchingResultUnenriched(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`{"results":[{"id":1,"title":"Different","original_title":"Different","release_date":"2024-01-01"}]}`))}, nil
	})}

	got, err := NewTMDBClient(client, "https://tmdb.test", "secret").Enrich(context.Background(), Movie{SearchName: "Bad Boys", Year: "1995"})

	require.NoError(t, err)
	require.Empty(t, got.ID)
}

func TestTMDBClientUsesRussianAliasOnlyAfterOriginalHasNoAcceptedCandidate(t *testing.T) {
	var queries []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		query := r.URL.Query().Get("query")
		queries = append(queries, query)
		body := `{"results":[]}`
		if query == "Магеллан" {
			body = `{"results":[{"id":2,"title":"Магеллан","original_title":"Magalhães","release_date":"2024-04-01"}]}`
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(body))}, nil
	})}

	got, err := NewTMDBClient(client, "https://tmdb.test", "secret").Enrich(context.Background(), Movie{
		SearchAliases: []string{"Magellan", "Магеллан"}, SearchName: "Magellan", Year: "2024",
	})

	require.NoError(t, err)
	require.Equal(t, "2", got.ID)
	require.Equal(t, []string{"Magellan", "Магеллан"}, queries)
}

func TestTMDBClientStopsAfterAcceptedOriginalAlias(t *testing.T) {
	var attempts atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts.Add(1)
		require.Equal(t, "Bad Boys", r.URL.Query().Get("query"))
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(
			`{"results":[{"id":9737,"title":"Bad Boys","original_title":"Bad Boys","release_date":"1995-04-07"}]}`,
		))}, nil
	})}

	got, err := NewTMDBClient(client, "https://tmdb.test", "secret").Enrich(context.Background(), Movie{
		SearchAliases: []string{"Bad Boys", "Плохие парни"}, SearchName: "Bad Boys", Year: "1995",
	})

	require.NoError(t, err)
	require.Equal(t, "9737", got.ID)
	require.EqualValues(t, 1, attempts.Load())
}

func TestTMDBRetryDelayIsCappedAtFiveSeconds(t *testing.T) {
	response := &http.Response{Header: http.Header{"Retry-After": []string{"60"}}}
	require.Equal(t, 5*time.Second, tmdbRetryDelay(response, time.Second))
}

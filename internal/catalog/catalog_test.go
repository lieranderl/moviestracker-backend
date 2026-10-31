package catalog

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseRutorHTMLReturnsStableMovieMetadata(t *testing.T) {
	html := `<table><tr class="gai"><td>06 Янв 24</td><td><a href="magnet:?xt=urn:btih:5A64F87D68A7E4DF9F0E3F26B4E4C65C09D6FFAA">magnet</a><a href="/torrent/1">Плохие парни / Bad Boys (2024) 2160p HDR10+</a></td><td>12.5 GB 10 2</td></tr></table>`

	got, err := ParseRutorHTML(strings.NewReader(html), false)

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "Bad Boys", got[0].OriginalName)
	require.Equal(t, "2024", got[0].Year)
	require.Equal(t, "5A64F87D68A7E4DF9F0E3F26B4E4C65C09D6FFAA", got[0].MagnetHash)
	require.True(t, got[0].K4)
	require.True(t, got[0].HDR10Plus)
}

func FuzzParseRutorHTML(f *testing.F) {
	f.Add([]byte(`<table><tr class="gai"><td>06 Янв 24</td><td><a href="magnet:?xt=urn:btih:ABC">m</a><a>Фильм / Movie (2024)</a></td><td>1 GB 2 1</td></tr></table>`), false)
	f.Add([]byte(`<html><broken`), true)
	f.Fuzz(func(t *testing.T, input []byte, series bool) {
		if len(input) > 1<<20 {
			t.Skip()
		}
		_, _ = ParseRutorHTML(strings.NewReader(string(input)), series)
	})
}

func TestRutorClientStopsBlockedRequestWhenContextIsCancelled(t *testing.T) {
	started := make(chan struct{})
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := NewRutorClient(client).Fetch(ctx, "https://tracker.test", false)
		errCh <- err
	}()

	<-started
	cancel()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("request did not stop after context cancellation")
	}
}

func TestRutorClientRejectsOversizedResponse(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(strings.Repeat(" ", 4<<20) + "<html></html>")),
		}, nil
	})}

	_, err := NewRutorClient(client).Fetch(context.Background(), "https://tracker.test", false)

	require.ErrorContains(t, err, "response exceeds")
}

func TestRutorClientAcceptsResponseAtSizeLimit(t *testing.T) {
	html := `<table><tr class="gai"><td>06 Янв 24</td><td><a href="magnet:?xt=urn:btih:5A64F87D68A7E4DF9F0E3F26B4E4C65C09D6FFAA">magnet</a><a href="/torrent/1">Плохие парни / Bad Boys (1995)</a></td><td>12.5 GB 10 2</td></tr></table>`
	body := html + strings.Repeat(" ", (4<<20)-len(html))
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}

	torrents, err := NewRutorClient(client).Fetch(context.Background(), "https://tracker.test", false)

	require.NoError(t, err)
	require.Len(t, torrents, 1)
}

func TestRutorClientRejectsChallengePage(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`<html><title>Just a moment</title><body>Verify you are human</body></html>`)),
		}, nil
	})}

	_, err := NewRutorClient(client).Fetch(context.Background(), "https://tracker.test", false)

	require.ErrorContains(t, err, "unexpected rutor document")
}

func TestRutorClientLimitsDecompressedResponse(t *testing.T) {
	var archive bytes.Buffer
	writer := gzip.NewWriter(&archive)
	_, err := io.WriteString(writer, strings.Repeat(" ", 4<<20)+"<html></html>")
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.Less(t, archive.Len(), 4<<20)
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		// net/http exposes a gzip-decoded body when it negotiates compression.
		reader, err := gzip.NewReader(bytes.NewReader(archive.Bytes()))
		require.NoError(t, err)
		return &http.Response{StatusCode: http.StatusOK, Body: reader, Uncompressed: true}, nil
	})}

	_, err = NewRutorClient(client).Fetch(context.Background(), "https://tracker.test", false)

	require.ErrorContains(t, err, "response exceeds")
}

// Titles below are taken from the live Rutor HDR10 and Dolby Vision searches
// on 2026-10-31; dubbing is credited as "D, P", never as "Dolby".
func TestParseRutorTitleFormatFlags(t *testing.T) {
	tests := map[string]struct {
		title         string
		dv, hdr10, k4 bool
	}{
		"dolby vision and hdr10+": {title: "Плохие парни / Bad Boys (2024) UHD BDRemux 2160p | 4K | HDR10+ | Dolby Vision | D, P", dv: true, hdr10: true, k4: true},
		"dolby vision profile":    {title: "Фильм / Movie (2025) WEB-DL 2160p | 4K | HDR | Dolby Vision Profile 8 | P", dv: true, k4: true},
		"dv abbreviation":         {title: "Фильм / Movie (2025) WEB-DL 2160p | HDR10, DV | P", dv: true, hdr10: true, k4: true},
		"dolby audio is not dv":   {title: "Фильм / Movie (2025) BDRip 1080p | Dolby Atmos | Dolby Digital", dv: false},
		"dvd is not dv":           {title: "Фильм / Movie (1999) DVD9 | DVDRip", dv: false},
		"no attribute section":    {title: "Матрица. Квадрология / The Matrix. Quadrology / 1999-2021 / UHD BDRemux 2160p | 4K | HDR | Dolby Vision P8 | D", dv: true, k4: true},
		"plain hdr10":             {title: "Фильм / Movie (2024) WEB-DL 2160p | HDR10 | P", hdr10: true, k4: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := parseRutorTitle(test.title)
			require.Equal(t, test.dv, got.DV, "DV")
			require.Equal(t, test.hdr10, got.HDR10, "HDR10")
			require.Equal(t, test.k4, got.K4, "2160p")
		})
	}
}

func TestCollectionAcceptsOnlyItsFormat(t *testing.T) {
	plain := Torrent{}
	hdr10 := Torrent{HDR10: true}
	dv := Torrent{DV: true}

	require.True(t, CollectionLatest.Accepts(plain))
	require.True(t, CollectionHDR10.Accepts(hdr10))
	require.False(t, CollectionHDR10.Accepts(dv))
	require.True(t, CollectionDV.Accepts(dv))
	require.False(t, CollectionDV.Accepts(hdr10))
}

func TestRunnerRejectsReleasesOutsideTheCollectionFormat(t *testing.T) {
	tracker := TrackerFunc(func(context.Context, string, bool) ([]Torrent, error) {
		return []Torrent{
			{Hash: "dv", OriginalName: "Vision", Year: "2024", DV: true},
			{Hash: "atmos", OriginalName: "Atmos only", Year: "2024"},
		}, nil
	})
	repository := &recordingRepository{}

	result, err := Runner{Tracker: tracker, Repository: repository}.Run(context.Background(), RunConfig{
		Collection: CollectionDV, URLs: []string{"https://dv.test"},
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.RejectedClassification)
	require.Equal(t, 1, result.Grouped)
	require.Len(t, repository.saved, 1)
	require.Equal(t, "Vision", repository.saved[0].SearchName)
}

func TestRutorClientRetriesTransientFailureOnce(t *testing.T) {
	page := `<table><tr class="gai"><td>06 Янв 24</td><td><a href="magnet:?xt=urn:btih:5A64F87D68A7E4DF9F0E3F26B4E4C65C09D6FFAA">magnet</a><a href="/torrent/1">Плохие парни / Bad Boys (1995)</a></td><td>12.5 GB 10 2</td></tr></table>`
	tests := map[string]struct {
		responses []int
		transport bool
		calls     int
		wantErr   string
	}{
		"recovers after 503":       {responses: []int{http.StatusServiceUnavailable, http.StatusOK}, calls: 2},
		"recovers after 429":       {responses: []int{http.StatusTooManyRequests, http.StatusOK}, calls: 2},
		"recovers after transport": {responses: []int{0, http.StatusOK}, calls: 2},
		"gives up after two":       {responses: []int{http.StatusBadGateway, http.StatusBadGateway}, calls: 2, wantErr: "502"},
		"does not retry 404":       {responses: []int{http.StatusNotFound}, calls: 1, wantErr: "404"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var mu sync.Mutex
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				mu.Lock()
				status := test.responses[calls]
				calls++
				mu.Unlock()
				if status == 0 {
					return nil, errors.New("connection reset")
				}
				return &http.Response{StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status)), Body: io.NopCloser(strings.NewReader(page))}, nil
			})}
			rutor := NewRutorClient(client)
			rutor.retryDelay = time.Millisecond

			torrents, err := rutor.Fetch(context.Background(), "https://tracker.test", false)

			require.Equal(t, test.calls, calls)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, torrents, 1)
		})
	}
}

func TestRutorClientDoesNotRetryChallengePage(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`<html><title>Just a moment</title></html>`))}, nil
	})}

	_, err := NewRutorClient(client).Fetch(context.Background(), "https://tracker.test", false)

	require.ErrorContains(t, err, "unexpected rutor document")
	require.Equal(t, 1, calls)
}

func TestRutorClientStopsRetryWaitOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		cancel()
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Status: "503 Service Unavailable", Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	rutor := NewRutorClient(client)
	rutor.retryDelay = time.Hour

	_, err := rutor.Fetch(ctx, "https://tracker.test", false)

	require.ErrorIs(t, err, context.Canceled)
}

func TestRutorParserPreservesStructuredPageWithoutRequestedMedia(t *testing.T) {
	html := `<table><tr class="gai"><td>06 Янв 24</td><td><a href="magnet:?xt=urn:btih:5A64F87D68A7E4DF9F0E3F26B4E4C65C09D6FFAA">magnet</a><a href="/torrent/1">Сериал / Series (2024) [S01]</a></td><td>12.5 GB 10 2</td></tr></table>`

	batch, err := ParseRutorHTMLWithDiagnostics(strings.NewReader(html), false)

	require.NoError(t, err)
	require.Empty(t, batch.Torrents)
	require.Zero(t, batch.RejectedParsing)
	require.Equal(t, 1, batch.SkippedSeries)
}

func TestRunnerDegradesWhenOneTrackerReturnsChallenge(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `<html><title>Just a moment</title></html>`
		if r.URL.Host == "working.test" {
			body = `<table><tr class="gai"><td>06 Янв 24</td><td><a href="magnet:?xt=urn:btih:5A64F87D68A7E4DF9F0E3F26B4E4C65C09D6FFAA">magnet</a><a href="/torrent/1">Плохие парни / Bad Boys (1995)</a></td><td>12.5 GB 10 2</td></tr></table>`
		}
		if r.URL.Host == "tmdb.test" {
			body = `{"results":[{"id":9737,"title":"Bad Boys","original_title":"Bad Boys","release_date":"1995-04-07"}]}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	repository := &recordingRepository{}
	runner := Runner{
		Tracker: NewRutorClient(client), Enricher: NewTMDBClient(client, "https://tmdb.test", "secret"), Repository: repository,
	}

	result, err := runner.Run(context.Background(), RunConfig{
		Collection: CollectionLatest, URLs: []string{"https://challenge.test", "https://working.test"},
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Failed)
	require.Equal(t, 1, result.Stored)
	require.Equal(t, 1, result.Written)
	require.Len(t, repository.saved, 1)
	require.Equal(t, "9737", repository.saved[0].ID)

	_, err = runner.Run(context.Background(), RunConfig{
		Collection: CollectionLatest, URLs: []string{"https://challenge.test"},
	})
	require.ErrorContains(t, err, "every tracker source failed")
}

func BenchmarkParseRutorHTML(b *testing.B) {
	row := `<tr class="gai"><td>06 Янв 24</td><td><a href="magnet:?xt=urn:btih:5A64F87D68A7E4DF9F0E3F26B4E4C65C09D6FFAA">magnet</a><a href="/torrent/1">Плохие парни / Bad Boys (1995)</a></td><td>12.5 GB 10 2</td></tr>`
	html := "<table>" + strings.Repeat(row, 100) + "</table>"
	b.ReportAllocs()
	b.SetBytes(int64(len(html)))
	for b.Loop() {
		if _, err := ParseRutorHTML(strings.NewReader(html), false); err != nil {
			b.Fatal(err)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestGroupMoviesDeduplicatesAndKeepsNewestTimestamp(t *testing.T) {
	torrents := []Torrent{
		{Hash: "movie-a", OriginalName: "Movie A", Year: "2024", Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Hash: "movie-a", OriginalName: "Movie A", Year: "2024", Date: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
	}

	got := GroupMovies(torrents)

	require.Len(t, got, 1)
	require.Equal(t, "Movie A", got[0].SearchName)
	require.Equal(t, []string{"Movie A"}, got[0].SearchAliases)
	require.Equal(t, torrents[1].Date, got[0].LastTimeFound)
}

func TestGroupMoviesKeepsOriginalThenDistinctRussianAlias(t *testing.T) {
	got := GroupMovies([]Torrent{{
		Hash: "movie-a", OriginalName: "The Movie", RussianName: "Фильм", Name: "ignored", Year: "2024",
	}})

	require.Len(t, got, 1)
	require.Equal(t, []string{"The Movie", "Фильм"}, got[0].SearchAliases)
	require.Equal(t, "The Movie", got[0].SearchName)

	duplicate := GroupMovies([]Torrent{{Hash: "movie-b", OriginalName: "Same", RussianName: " same "}})
	require.Equal(t, []string{"Same"}, duplicate[0].SearchAliases)
}

func TestResultResolvedRatio(t *testing.T) {
	require.InDelta(t, 0.95, Result{Grouped: 20, Resolved: 19}.ResolvedRatio(), 1e-9)
	require.Equal(t, 1.0, Result{Grouped: 3, Resolved: 3}.ResolvedRatio())
	require.Zero(t, Result{}.ResolvedRatio(), "a run that grouped nothing is reported as fully unresolved")
}

func TestRunnerPassesCallerContextToEveryBoundary(t *testing.T) {
	cancelled := errors.New("cancelled at tracker")
	tracker := TrackerFunc(func(ctx context.Context, _ string, _ bool) ([]Torrent, error) {
		return nil, cancelled
	})
	runner := Runner{Tracker: tracker}

	_, err := runner.Run(context.Background(), RunConfig{URLs: []string{"https://tracker.example"}, Collection: CollectionLatest})

	require.ErrorIs(t, err, cancelled)
}

// Losing one tracker page must not discard the pages that succeeded. The DV and
// HDR10 collections scrape five and four pages respectively.
func TestRunnerKeepsWorkingTrackerSourcesWhenOneFails(t *testing.T) {
	tracker := TrackerFunc(func(_ context.Context, url string, _ bool) ([]Torrent, error) {
		if url == "https://broken.test" {
			return nil, errors.New("502 bad gateway")
		}
		return []Torrent{{Hash: url, OriginalName: "Movie " + url, Year: "2024", DV: true}}, nil
	})
	repository := &recordingRepository{}
	runner := Runner{Tracker: tracker, Repository: repository}

	result, err := runner.Run(context.Background(), RunConfig{
		Collection: CollectionDV,
		URLs:       []string{"https://one.test", "https://broken.test", "https://two.test"},
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Failed)
	require.Equal(t, 2, result.Fetched)
	require.Equal(t, 2, result.Grouped)
	require.Len(t, repository.saved, 2)
}

func TestRunnerFailsWhenEveryTrackerSourceFails(t *testing.T) {
	tracker := TrackerFunc(func(context.Context, string, bool) ([]Torrent, error) {
		return nil, errors.New("dns failure")
	})
	repository := &recordingRepository{}
	runner := Runner{Tracker: tracker, Repository: repository}

	_, err := runner.Run(context.Background(), RunConfig{
		Collection: CollectionDV,
		URLs:       []string{"https://one.test", "https://two.test"},
	})

	require.ErrorContains(t, err, "every tracker source failed")
	require.Nil(t, repository.saved, "nothing may be written when the whole scrape failed")
}

// A transient enrichment error must cost one title, not the whole run.
func TestRunnerPersistsMoviesThatEnrichedWhenSomeFail(t *testing.T) {
	tracker := TrackerFunc(func(context.Context, string, bool) ([]Torrent, error) {
		torrents := make([]Torrent, 0, 10)
		for index := range 10 {
			name := fmt.Sprintf("Movie %d", index)
			torrents = append(torrents, Torrent{Hash: name, OriginalName: name, Year: "2024"})
		}
		return torrents, nil
	})
	enricher := enricherFunc(func(_ context.Context, movie Movie) (Movie, error) {
		if movie.SearchName == "Movie 3" {
			return movie, errors.New("tmdb timeout")
		}
		movie.ID = movie.SearchName
		return movie, nil
	})
	repository := &recordingRepository{}
	runner := Runner{Tracker: tracker, Enricher: enricher, Repository: repository}

	result, err := runner.Run(context.Background(), RunConfig{Collection: CollectionLatest, URLs: []string{"https://one.test"}})

	require.NoError(t, err)
	require.Equal(t, 10, result.Grouped)
	require.Equal(t, 9, result.Resolved)
	require.Equal(t, 1, result.Failed)
	require.Len(t, repository.saved, 9)
	for _, movie := range repository.saved {
		require.NotEqual(t, "Movie 3", movie.SearchName)
	}
}

// A rejected API key or a TMDB outage fails nearly everything. That must fail
// loudly instead of quietly replacing the catalog with a near-empty one.
func TestRunnerFailsWhenMostEnrichmentFails(t *testing.T) {
	tracker := TrackerFunc(func(context.Context, string, bool) ([]Torrent, error) {
		torrents := make([]Torrent, 0, 10)
		for index := range 10 {
			name := fmt.Sprintf("Movie %d", index)
			torrents = append(torrents, Torrent{Hash: name, OriginalName: name, Year: "2024"})
		}
		return torrents, nil
	})
	enricher := enricherFunc(func(_ context.Context, movie Movie) (Movie, error) {
		return movie, errors.New("401 invalid api key")
	})
	repository := &recordingRepository{}
	runner := Runner{Tracker: tracker, Enricher: enricher, Repository: repository}

	_, err := runner.Run(context.Background(), RunConfig{Collection: CollectionLatest, URLs: []string{"https://one.test"}})

	require.ErrorContains(t, err, "enrichment failed for 10 of 10 movies")
	require.ErrorContains(t, err, "401 invalid api key")
	require.Nil(t, repository.saved)
}

// Movies TMDB simply does not know are not failures; they are dropped quietly,
// as before, because Firestore documents are keyed on the TMDB id.
func TestRunnerDropsUnresolvedMoviesWithoutFailing(t *testing.T) {
	tracker := TrackerFunc(func(context.Context, string, bool) ([]Torrent, error) {
		return []Torrent{
			{Hash: "a", OriginalName: "Known", Year: "2024"},
			{Hash: "b", OriginalName: "Unknown", Year: "2024"},
		}, nil
	})
	enricher := enricherFunc(func(_ context.Context, movie Movie) (Movie, error) {
		if movie.SearchName == "Known" {
			movie.ID = "42"
		}
		return movie, nil
	})
	repository := &recordingRepository{}
	runner := Runner{Tracker: tracker, Enricher: enricher, Repository: repository}

	result, err := runner.Run(context.Background(), RunConfig{Collection: CollectionLatest, URLs: []string{"https://one.test"}})

	require.NoError(t, err)
	require.Equal(t, 0, result.Failed)
	require.Equal(t, 1, result.Resolved)
	require.Equal(t, 1, result.Unresolved)
	require.Len(t, repository.saved, 1)
	require.Equal(t, "42", repository.saved[0].ID)
}

func TestRunnerReportsCancellationRatherThanPartialSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tracker := TrackerFunc(func(context.Context, string, bool) ([]Torrent, error) {
		return []Torrent{{Hash: "a", OriginalName: "Movie", Year: "2024"}}, nil
	})
	enricher := enricherFunc(func(_ context.Context, movie Movie) (Movie, error) {
		cancel()
		return movie, context.Canceled
	})
	repository := &recordingRepository{}
	runner := Runner{Tracker: tracker, Enricher: enricher, Repository: repository}

	_, err := runner.Run(ctx, RunConfig{Collection: CollectionLatest, URLs: []string{"https://one.test"}})

	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, repository.saved)
}

func TestRunnerValidatesEnricherConcurrency(t *testing.T) {
	runner := Runner{Tracker: TrackerFunc(func(context.Context, string, bool) ([]Torrent, error) { return nil, nil })}
	for _, value := range []int{-1, 21} {
		_, err := runner.Run(context.Background(), RunConfig{
			Collection: CollectionLatest, EnricherConcurrency: value, URLs: []string{"https://one.test"},
		})
		require.ErrorContains(t, err, "enricher concurrency")
	}
	require.Equal(t, 10, defaultEnricherConcurrency)
	require.Equal(t, 20, maxEnricherConcurrency)
}

func TestParseRutorHTMLRejectsMalformedDateWithDiagnostics(t *testing.T) {
	html := `<table><tr class="gai"><td>not-a-date</td><td><a href="magnet:?xt=urn:btih:5A64F87D68A7E4DF9F0E3F26B4E4C65C09D6FFAA">magnet</a><a href="/torrent/1">Фильм / Movie (2024) 2160p</a></td><td>12.5 GB 10 2</td></tr></table>`

	batch, err := ParseRutorHTMLWithDiagnostics(strings.NewReader(html), false)

	require.NoError(t, err)
	require.Equal(t, 1, batch.Fetched)
	require.Equal(t, 1, batch.RejectedParsing)
	require.Empty(t, batch.Torrents)
}

type enricherFunc func(context.Context, Movie) (Movie, error)

func (f enricherFunc) Enrich(ctx context.Context, movie Movie) (Movie, error) { return f(ctx, movie) }

type recordingRepository struct {
	mu    sync.Mutex
	saved []Movie
}

func (r *recordingRepository) Upsert(_ context.Context, _ Collection, movies []Movie) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saved = append(r.saved, movies...)
	return len(movies), nil
}

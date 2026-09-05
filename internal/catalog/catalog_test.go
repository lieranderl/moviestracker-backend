package catalog

import (
	"context"
	"errors"
	"net/http"
	"strings"
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
	require.Equal(t, torrents[1].Date, got[0].LastTimeFound)
}

func TestRunnerPassesCallerContextToEveryBoundary(t *testing.T) {
	cancelled := errors.New("cancelled at tracker")
	tracker := TrackerFunc(func(ctx context.Context, _ string, _ bool) ([]Torrent, error) {
		return nil, cancelled
	})
	runner := Runner{Tracker: tracker}

	err := runner.Run(context.Background(), RunConfig{URLs: []string{"https://tracker.example"}, Collection: CollectionLatest})

	require.ErrorIs(t, err, cancelled)
}

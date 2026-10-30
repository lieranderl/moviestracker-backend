package imdb

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func loadedManager(t *testing.T, raw string) *Manager {
	t.Helper()
	manager := NewManager(0)
	require.NoError(t, manager.Refresh(context.Background(), SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{Generation: 1, Body: io.NopCloser(strings.NewReader(raw))}, nil
	})))
	return manager
}

// Start-up already loads the index, so the first interval must elapse before the
// request path spends anything on a freshness check.
func TestRefresherDoesNotCheckBeforeTheFirstIntervalElapses(t *testing.T) {
	var opens atomic.Int32
	manager := loadedManager(t, "tt0000001\t7.0\t10\n")
	refresher := NewRefresher(manager, SourceFunc(func(context.Context, int64) (Snapshot, error) {
		opens.Add(1)
		return Snapshot{Generation: 1, Unchanged: true}, nil
	}), time.Hour)

	for range 50 {
		refresher.Ensure(context.Background())
	}

	require.Zero(t, opens.Load())
}

func TestRefresherChecksOncePerIntervalNotPerRequest(t *testing.T) {
	var opens atomic.Int32
	manager := loadedManager(t, "tt0000001\t7.0\t10\n")
	refresher := NewRefresher(manager, SourceFunc(func(context.Context, int64) (Snapshot, error) {
		opens.Add(1)
		return Snapshot{Generation: 1, Unchanged: true}, nil
	}), time.Millisecond)

	time.Sleep(3 * time.Millisecond)
	for range 100 {
		refresher.Ensure(context.Background())
	}

	// The first due request performs the check and records the attempt; the other
	// ninety-nine fall inside the fresh interval.
	require.EqualValues(t, 1, opens.Load())
}

func TestRefresherPicksUpANewGeneration(t *testing.T) {
	manager := loadedManager(t, "tt0000001\t7.0\t10\n")
	refresher := NewRefresher(manager, SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{Generation: 2, Body: io.NopCloser(strings.NewReader("tt0000001\t9.9\t99\n"))}, nil
	}), time.Millisecond)

	time.Sleep(3 * time.Millisecond)
	refresher.Ensure(context.Background())

	require.EqualValues(t, 2, manager.Generation())
	rating, ok := manager.Lookup("tt0000001")
	require.True(t, ok)
	require.Equal(t, "9.9", rating.Rating)
}

// A failing source must not take the service down or empty the index, and must
// not be retried on every single request.
func TestRefresherRetainsIndexAndBacksOffOnFailure(t *testing.T) {
	var opens atomic.Int32
	manager := loadedManager(t, "tt0000001\t7.0\t10\n")
	refresher := NewRefresher(manager, SourceFunc(func(context.Context, int64) (Snapshot, error) {
		opens.Add(1)
		return Snapshot{}, errors.New("storage unavailable")
	}), time.Hour)
	refresher.lastAttempt.Store(0) // make a check due immediately

	for range 20 {
		refresher.Ensure(context.Background())
	}

	require.EqualValues(t, 1, opens.Load(), "a failing source must not be retried on every request")
	rating, ok := manager.Lookup("tt0000001")
	require.True(t, ok)
	require.Equal(t, "7.0", rating.Rating)
}

// Concurrent requests must not each start a rebuild.
func TestRefresherAdmitsOnlyOneRefreshAtATime(t *testing.T) {
	var concurrent, peak, opens atomic.Int32
	release := make(chan struct{})
	manager := loadedManager(t, "tt0000001\t7.0\t10\n")
	refresher := NewRefresher(manager, SourceFunc(func(context.Context, int64) (Snapshot, error) {
		opens.Add(1)
		if current := concurrent.Add(1); current > peak.Load() {
			peak.Store(current)
		}
		<-release
		concurrent.Add(-1)
		return Snapshot{Generation: 1, Unchanged: true}, nil
	}), time.Hour)
	refresher.lastAttempt.Store(0)

	var waiting sync.WaitGroup
	for range 16 {
		waiting.Add(1)
		go func() {
			defer waiting.Done()
			refresher.Ensure(context.Background())
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	waiting.Wait()

	require.EqualValues(t, 1, peak.Load(), "refreshes must not overlap; two live indexes double memory")
	require.EqualValues(t, 1, opens.Load())
}

// Requests arriving during a refresh are served immediately from the current
// index rather than blocked behind it.
func TestRefresherDoesNotBlockConcurrentRequests(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{})
	manager := loadedManager(t, "tt0000001\t7.0\t10\n")
	refresher := NewRefresher(manager, SourceFunc(func(context.Context, int64) (Snapshot, error) {
		close(entered)
		<-release
		return Snapshot{Generation: 1, Unchanged: true}, nil
	}), time.Hour)
	refresher.lastAttempt.Store(0)

	go refresher.Ensure(context.Background())
	<-entered

	served := make(chan struct{})
	go func() {
		refresher.Ensure(context.Background())
		close(served)
	}()

	select {
	case <-served:
	case <-time.After(time.Second):
		t.Fatal("a request blocked waiting for an in-flight refresh")
	}
	close(release)
}

func TestRefresherIsDisabledByANonPositiveInterval(t *testing.T) {
	var opens atomic.Int32
	manager := loadedManager(t, "tt0000001\t7.0\t10\n")
	refresher := NewRefresher(manager, SourceFunc(func(context.Context, int64) (Snapshot, error) {
		opens.Add(1)
		return Snapshot{Generation: 1, Unchanged: true}, nil
	}), 0)
	refresher.lastAttempt.Store(0)

	refresher.Ensure(context.Background())

	require.Zero(t, opens.Load())
}

func TestRefresherMiddlewareServesTheRequestAndRefreshes(t *testing.T) {
	var opens atomic.Int32
	manager := loadedManager(t, "tt0000001\t7.0\t10\n")
	refresher := NewRefresher(manager, SourceFunc(func(context.Context, int64) (Snapshot, error) {
		opens.Add(1)
		return Snapshot{Generation: 2, Body: io.NopCloser(strings.NewReader("tt0000001\t8.8\t20\n"))}, nil
	}), time.Millisecond)
	handler := refresher.Middleware(NewHandler(manager.Lookup))

	time.Sleep(3 * time.Millisecond)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/getimdb?imdb_id=tt0000001", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.EqualValues(t, 1, opens.Load())
	// The refresh completes before the lookup, so the request observes new data.
	require.JSONEq(t, `{"Id":"tt0000001","Rating":"8.8","Votes":"20"}`, recorder.Body.String())
}

func TestRefresherBoundsStorageWorkAndServesExistingRating(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		manager := loadedManager(t, "tt0000001\t7.0\t10\n")
		refresher := NewRefresher(manager, SourceFunc(func(ctx context.Context, _ int64) (Snapshot, error) {
			deadline, bounded := ctx.Deadline()
			require.True(t, bounded, "storage work needs its own deadline")
			require.LessOrEqual(t, time.Until(deadline), 10*time.Second)
			<-ctx.Done()
			return Snapshot{}, ctx.Err()
		}), time.Hour)
		refresher.lastAttempt.Store(0)
		response := httptest.NewRecorder()
		start := time.Now()

		refresher.Middleware(NewHandler(manager.Lookup)).ServeHTTP(response,
			httptest.NewRequest(http.MethodGet, "/getimdb?imdb_id=tt0000001", nil))

		require.Equal(t, http.StatusOK, response.Code)
		require.JSONEq(t, `{"Id":"tt0000001","Rating":"7.0","Votes":"10"}`, response.Body.String())
		require.EqualValues(t, 1, manager.Generation())
		require.Equal(t, 10*time.Second, time.Since(start))
	})
}

func TestRefreshNowCancelsWhileAnotherRefreshIsRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		manager := loadedManager(t, "tt0000001\t7.0\t10\n")
		entered, finished := make(chan struct{}), make(chan struct{})
		refresher := NewRefresher(manager, SourceFunc(func(ctx context.Context, _ int64) (Snapshot, error) {
			close(entered)
			<-ctx.Done()
			return Snapshot{}, ctx.Err()
		}), time.Hour)
		refresher.lastAttempt.Store(0)
		owner, release := context.WithCancel(t.Context())
		defer release()
		go func() { refresher.Ensure(owner); close(finished) }()
		<-entered
		waiter, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		require.ErrorIs(t, refresher.RefreshNow(waiter), context.DeadlineExceeded)
		release()
		<-finished
	})
}

func TestRefreshHandlerPublishesNewGeneration(t *testing.T) {
	manager := loadedManager(t, "tt0000001\t7.0\t10\n")
	refresher := NewRefresher(manager, SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{Generation: 2, Body: io.NopCloser(strings.NewReader("tt0000001\t9.1\t30\n"))}, nil
	}), time.Hour)
	recorder := httptest.NewRecorder()

	NewRefreshHandler(refresher).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/internal/refresh", nil))

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.EqualValues(t, 2, manager.Generation())
}

func TestRefreshHandlerRejectsWrongMethodAndHidesSourceError(t *testing.T) {
	manager := loadedManager(t, "tt0000001\t7.0\t10\n")
	refresher := NewRefresher(manager, SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{}, errors.New("secret storage detail")
	}), time.Hour)

	wrongMethod := httptest.NewRecorder()
	NewRefreshHandler(refresher).ServeHTTP(wrongMethod, httptest.NewRequest(http.MethodGet, "/internal/refresh", nil))
	require.Equal(t, http.StatusMethodNotAllowed, wrongMethod.Code)

	failed := httptest.NewRecorder()
	NewRefreshHandler(refresher).ServeHTTP(failed, httptest.NewRequest(http.MethodPost, "/internal/refresh", nil))
	require.Equal(t, http.StatusServiceUnavailable, failed.Code)
	require.NotContains(t, failed.Body.String(), "secret storage detail")
}

func TestManagerLenReportsLiveRowCount(t *testing.T) {
	require.Zero(t, NewManager(0).Len())
	require.Equal(t, 2, loadedManager(t, "tt0000001\t7.0\t10\ntt0000002\t8.0\t20\n").Len())
}

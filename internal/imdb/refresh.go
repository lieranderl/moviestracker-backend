package imdb

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Refresher keeps the index current without a background timer.
//
// The service runs with cpu_idle enabled and min_instance_count 0, so CPU is
// only allocated while a request is being served. A time.Ticker goroutine is
// therefore not reliably scheduled: the previous background refresh could sit
// unscheduled indefinitely, and the alert watching it could never fire. Doing
// the work on the request path is the only mechanism that runs under those
// settings without paying for an always-warm instance.
//
// The cost is bounded because Source.Open compares the Cloud Storage object
// generation first and reports Unchanged without transferring the object. A due
// check is one metadata call; a full rebuild only happens when the download job
// has actually published a new generation, which is once a day.
type Refresher struct {
	interval time.Duration
	// lastAttempt is the Unix nanosecond time of the last attempt, recorded
	// before the attempt so a persistently failing source is not retried on
	// every request.
	lastAttempt atomic.Int64
	manager     *Manager
	// refreshing admits one refresh at a time. Requests that arrive while a
	// refresh is running are served from the current index rather than queued.
	refreshing sync.Mutex
	source     Source
}

func NewRefresher(manager *Manager, source Source, interval time.Duration) *Refresher {
	refresher := &Refresher{interval: interval, manager: manager, source: source}
	// The caller loads the initial index before serving, so the first
	// opportunistic check is due one interval after start-up.
	refresher.lastAttempt.Store(time.Now().UnixNano())
	return refresher
}

// Ensure refreshes the index when a check is due, at most once per interval and
// never concurrently. A failure is logged and the current index is retained.
func (r *Refresher) Ensure(ctx context.Context) {
	if !r.due(time.Now()) {
		return
	}
	if !r.refreshing.TryLock() {
		return
	}
	defer r.refreshing.Unlock()
	// Re-check under the lock: several requests can pass the due test before the
	// winner records its attempt.
	now := time.Now()
	if !r.due(now) {
		return
	}
	r.lastAttempt.Store(now.UnixNano())
	if err := r.refresh(ctx); err != nil {
		slog.Error("IMDb index refresh failed; retaining current snapshot",
			"event", "imdb_index_refresh_failed",
			"error", err,
		)
	}
}

// RefreshNow synchronously checks the published generation. Cloud Run IAM
// protects the internal HTTP endpoint that calls this method.
func (r *Refresher) RefreshNow(ctx context.Context) error {
	r.refreshing.Lock()
	defer r.refreshing.Unlock()
	r.lastAttempt.Store(time.Now().UnixNano())
	return r.refresh(ctx)
}

func (r *Refresher) refresh(ctx context.Context) error {
	previous := r.manager.Generation()
	if err := r.manager.Refresh(ctx, r.source); err != nil {
		return err
	}
	if current := r.manager.Generation(); current != previous {
		slog.Info("IMDb index refreshed",
			"event", "imdb_index_refreshed",
			"generation", current,
			"previous_generation", previous,
			"rows", r.manager.Len(),
		)
	}
	return nil
}

func NewRefreshHandler(refresher *Refresher) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := refresher.RefreshNow(request.Context()); err != nil {
			slog.Error("IMDb index refresh failed",
				"event", "imdb_index_refresh_failed",
				"error", err,
			)
			http.Error(writer, "refresh failed", http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
}

func (r *Refresher) due(now time.Time) bool {
	if r.interval <= 0 {
		return false
	}
	return now.UnixNano()-r.lastAttempt.Load() >= int64(r.interval)
}

// Middleware runs the freshness check before delegating. It is applied to the
// lookup routes only, so probes never trigger storage access.
func (r *Refresher) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		r.Ensure(request.Context())
		next.ServeHTTP(writer, request)
	})
}

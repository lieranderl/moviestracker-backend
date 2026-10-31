package firestorestore

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/lieranderl/moviestracker-backend/internal/catalog"
	"github.com/stretchr/testify/require"
)

func TestEmulatorUpsertPreservesNewestDiscoveryAcrossConcurrentJobs(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST is not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	client, err := firestore.NewClientWithDatabase(ctx, "demo-moviestracker", "moviestracker")
	require.NoError(t, err)
	defer func() { require.NoError(t, client.Close()) }()
	repository := New(client)
	id := fmt.Sprintf("monotonic-%d", time.Now().UnixNano())
	reference := client.Collection(string(catalog.CollectionLatest)).Doc(id)
	defer func() { _, err := reference.Delete(ctx); require.NoError(t, err) }()

	initial := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	written, err := repository.Upsert(ctx, catalog.CollectionLatest, []catalog.Movie{{ID: id, LastTimeFound: initial}})
	require.NoError(t, err)
	require.Equal(t, 1, written)
	written, err = repository.Upsert(ctx, catalog.CollectionLatest, []catalog.Movie{{ID: id, LastTimeFound: initial.Add(-time.Hour)}})
	require.NoError(t, err)
	require.Zero(t, written, "an older discovery leaves the stored document unchanged")
	snapshot, err := reference.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, initial, snapshot.Data()["lasttimefound"])

	start := make(chan struct{})
	errors := make(chan error, 4)
	for _, offset := range []time.Duration{-2 * time.Hour, 3 * time.Hour, time.Hour, -time.Hour} {
		go func() {
			<-start
			_, err := repository.Upsert(ctx, catalog.CollectionLatest, []catalog.Movie{{ID: id, LastTimeFound: initial.Add(offset)}})
			errors <- err
		}()
	}
	close(start)
	results := make([]error, 0, 4)
	for range 4 {
		results = append(results, <-errors)
	}
	for _, err := range results {
		require.NoError(t, err)
	}
	snapshot, err = reference.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 10, 30, 15, 0, 0, 0, time.UTC), snapshot.Data()["lasttimefound"])
}

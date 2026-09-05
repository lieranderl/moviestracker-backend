package imdb

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRatingsBuildsLookupIndex(t *testing.T) {
	raw := "tconst\taverageRating\tnumVotes\ntt0133093\t8.7\t2100000\n"

	index, err := ParseRatings(strings.NewReader(raw))

	require.NoError(t, err)
	rating, ok := index.Lookup("tt0133093")
	require.True(t, ok)
	require.Equal(t, Rating{ID: "tt0133093", Rating: "8.7", Votes: "2100000"}, rating)
}

func TestManagerRetainsPreviousSnapshotWhenRefreshFails(t *testing.T) {
	manager := NewManager()
	good := SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{Generation: 1, Reader: strings.NewReader("tt1\t7.0\t10\n")}, nil
	})
	require.NoError(t, manager.Refresh(context.Background(), good))

	failed := SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{}, errors.New("storage unavailable")
	})
	err := manager.Refresh(context.Background(), failed)

	require.Error(t, err)
	rating, ok := manager.Lookup("tt1")
	require.True(t, ok)
	require.Equal(t, "7.0", rating.Rating)
}

func TestManagerSkipsUnchangedGeneration(t *testing.T) {
	manager := NewManager()
	calls := 0
	source := SourceFunc(func(_ context.Context, currentGeneration int64) (Snapshot, error) {
		calls++
		if currentGeneration == 7 {
			return Snapshot{Generation: 7, Unchanged: true}, nil
		}
		return Snapshot{Generation: 7, Reader: strings.NewReader("tt1\t7.0\t10\n")}, nil
	})

	require.NoError(t, manager.Refresh(context.Background(), source))
	require.NoError(t, manager.Refresh(context.Background(), source))
	require.Equal(t, 2, calls)
	require.Equal(t, int64(7), manager.Generation())
}

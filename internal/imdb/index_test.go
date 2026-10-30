package imdb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func body(raw string) io.ReadCloser { return io.NopCloser(strings.NewReader(raw)) }

func TestParseRatingsBuildsLookupIndex(t *testing.T) {
	raw := "tconst\taverageRating\tnumVotes\ntt0133093\t8.7\t2100000\n"

	index, err := ParseRatings(strings.NewReader(raw))

	require.NoError(t, err)
	rating, ok := index.Lookup("tt0133093")
	require.True(t, ok)
	require.Equal(t, Rating{ID: "tt0133093", Rating: "8.7", Votes: "2100000"}, rating)
	require.Equal(t, 1, index.Len())
}

func FuzzParseRatings(f *testing.F) {
	f.Add([]byte("tconst\taverageRating\tnumVotes\ntt0133093\t8.7\t2100000\n"))
	f.Add([]byte("bad\trow\n"))
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 1<<20 {
			t.Skip()
		}
		_, _ = ParseRatings(strings.NewReader(string(input)))
	})
}

// The published strings are part of the frozen HTTP contract, so every field has
// to survive the round trip through the integer encoding byte for byte.
func TestParseRatingsRoundTripsPublishedFormats(t *testing.T) {
	rows := []Rating{
		{ID: "tt0000001", Rating: "5.7", Votes: "2230"}, // minimum width, zero padded
		{ID: "tt0133093", Rating: "8.7", Votes: "2275256"},
		{ID: "tt0111161", Rating: "9.3", Votes: "3233728"}, // highest observed vote count
		{ID: "tt45115568", Rating: "1.0", Votes: "5"},      // highest observed identifier, lowest rating
		{ID: "tt10000000", Rating: "10.0", Votes: "0"},     // rating ceiling, eight digits, zero votes
		{ID: "tt9999999", Rating: "6.0", Votes: "1"},       // seven-digit boundary
	}
	var builder strings.Builder
	builder.WriteString("tconst\taverageRating\tnumVotes\n")
	for _, row := range rows {
		fmt.Fprintf(&builder, "%s\t%s\t%s\n", row.ID, row.Rating, row.Votes)
	}

	index, err := ParseRatings(strings.NewReader(builder.String()))
	require.NoError(t, err)

	require.Equal(t, len(rows), index.Len())
	for _, want := range rows {
		got, ok := index.Lookup(want.ID)
		require.Truef(t, ok, "expected %s to resolve", want.ID)
		require.Equal(t, want, got)
	}
}

// Compacting identifiers into integers must not quietly widen what resolves.
// The map-based index it replaces keyed on the literal string, so an unpadded or
// otherwise non-canonical identifier has to keep returning not-found.
func TestLookupRejectsNonCanonicalIdentifiers(t *testing.T) {
	index, err := ParseRatings(strings.NewReader("tt0133093\t8.7\t10\n"))
	require.NoError(t, err)

	for _, id := range []string{
		"tt133093",               // correct number, missing zero padding
		"tt00133093",             // over-padded
		"tt0133093 ",             // trailing space
		"0133093",                // missing prefix
		"tt",                     // no digits
		"ttabcdefg",              // non-numeric
		"tt99999999999999999999", // overflows uint32
	} {
		t.Run(id, func(t *testing.T) {
			_, ok := index.Lookup(id)
			require.False(t, ok)
		})
	}
}

// Rows outside the compact encoding must degrade into extra memory, never into a
// wrong or missing answer.
func TestParseRatingsPreservesRowsOutsideCompactEncoding(t *testing.T) {
	raw := strings.Join([]string{
		"tt0133093\t8.7\t10",          // compact
		"tt99999999999\t7.5\t20",      // identifier exceeds uint32
		"tt0000002\t7.25\t30",         // two decimal places
		"tt0000003\t6.1\t99999999999", // vote count exceeds uint32
		"",
	}, "\n")

	index, err := ParseRatings(strings.NewReader(raw))
	require.NoError(t, err)

	require.Equal(t, 4, index.Len())
	for _, want := range []Rating{
		{ID: "tt0133093", Rating: "8.7", Votes: "10"},
		{ID: "tt99999999999", Rating: "7.5", Votes: "20"},
		{ID: "tt0000002", Rating: "7.25", Votes: "30"},
		{ID: "tt0000003", Rating: "6.1", Votes: "99999999999"},
	} {
		got, ok := index.Lookup(want.ID)
		require.Truef(t, ok, "expected %s to resolve", want.ID)
		require.Equal(t, want, got)
	}
}

func TestParseRatingsSkipsMalformedRows(t *testing.T) {
	raw := "tconst\taverageRating\tnumVotes\ntt0000001\t5.7\t10\nbroken\ntt0000002\t5.7\t10\t extra\n\ntt0000003\t6.0\t7\n"

	index, stats, err := ParseRatingsWithStats(strings.NewReader(raw))

	require.NoError(t, err)
	require.Equal(t, 2, index.Len())
	require.Equal(t, 3, stats.SkippedRows, "the header is not a skipped row")
	_, ok := index.Lookup("tt0000002")
	require.False(t, ok)
}

// Entries are binary searched, so ordering has to hold regardless of input order
// and across the seven to eight digit boundary where lexical and numeric order
// disagree: "tt10000000" sorts before "tt2000000" as text but after it as a
// number, and the published export is sorted as text.
func TestParseRatingsOrdersEntriesNumerically(t *testing.T) {
	raw := "tt10000000\t7.0\t1\ntt2000000\t8.0\t2\ntt0000001\t9.0\t3\ntt45115568\t6.0\t4\n"

	index, err := ParseRatings(strings.NewReader(raw))
	require.NoError(t, err)

	require.Equal(t, []uint32{1, 2000000, 10000000, 45115568}, func() []uint32 {
		ids := make([]uint32, 0, len(index.entries))
		for _, e := range index.entries {
			ids = append(ids, e.id)
		}
		return ids
	}())
	for id, wantRating := range map[string]string{
		"tt0000001": "9.0", "tt2000000": "8.0", "tt10000000": "7.0", "tt45115568": "6.0",
	} {
		got, ok := index.Lookup(id)
		require.Truef(t, ok, "expected %s to resolve", id)
		require.Equal(t, wantRating, got.Rating)
	}
}

func TestParseRatingsSkipsSortForOrderedInput(t *testing.T) {
	index, stats, err := ParseRatingsWithStats(strings.NewReader(
		"tt0000001\t7.0\t1\ntt0000002\t8.0\t2\ntt10000000\t9.0\t3\n",
	))

	require.NoError(t, err)
	require.True(t, stats.InputOrdered)
	require.False(t, stats.SortApplied)
	require.Equal(t, 3, stats.CompactRows)
	require.Equal(t, []uint32{1, 2, 10000000}, []uint32{
		index.entries[0].id, index.entries[1].id, index.entries[2].id,
	})
}

func TestParseRatingsSortsUnorderedInputAsFallback(t *testing.T) {
	index, stats, err := ParseRatingsWithStats(strings.NewReader(
		"tt10000000\t9.0\t3\ntt0000002\t8.0\t2\ntt0000001\t7.0\t1\n",
	))

	require.NoError(t, err)
	require.False(t, stats.InputOrdered)
	require.True(t, stats.SortApplied)
	require.Equal(t, []uint32{1, 2, 10000000}, []uint32{
		index.entries[0].id, index.entries[1].id, index.entries[2].id,
	})
}

func TestScaledRatingBoundaries(t *testing.T) {
	for _, test := range []struct {
		raw    string
		scaled uint8
		ok     bool
	}{
		{"0.0", 0, true},
		{"1.0", 10, true},
		{"8.7", 87, true},
		{"10.0", 100, true},
		{"25.5", 255, true}, // uint8 ceiling
		{"25.6", 0, false},  // one past the ceiling
		{"8", 0, false},     // no decimal point
		{"8.75", 0, false},  // two decimals
		{".7", 0, false},    // no whole part
		{"8.", 0, false},    // no fraction
		{"a.7", 0, false},
	} {
		t.Run(test.raw, func(t *testing.T) {
			scaled, ok := parseScaledRating([]byte(test.raw))
			require.Equal(t, test.ok, ok)
			if test.ok {
				require.Equal(t, test.scaled, scaled)
				require.Equal(t, test.raw, formatScaledRating(scaled))
			}
		})
	}
}

func TestCanonicalTitleIDRoundTrip(t *testing.T) {
	for _, id := range []string{"tt0000001", "tt0133093", "tt9999999", "tt10000000", "tt45115568"} {
		numeric, ok := parseCanonicalTitleID([]byte(id))
		require.Truef(t, ok, "expected %s to parse", id)
		require.Equal(t, id, formatTitleID(numeric))
	}
	_, ok := parseCanonicalTitleID([]byte("tt" + strings.Repeat("9", 11)))
	require.False(t, ok, "identifiers beyond uint32 must be rejected")
	require.LessOrEqual(t, uint64(45115568), uint64(math.MaxUint32))
}

// The service runs in a 512 MiB Cloud Run container and Refresh holds the
// outgoing index while building the replacement, so the steady-state cost of one
// index is the number that matters. The map-based predecessor measured ~180 MiB
// for the real 1.71M rows, which peaked at ~479 MiB across a refresh.
func TestIndexMemoryBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates a full-size index")
	}
	const rows = 1_713_579
	const budgetMiB = 40

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	index, err := ParseRatings(syntheticRatings(rows))
	require.NoError(t, err)
	runtime.GC()
	runtime.ReadMemStats(&after)

	residentMiB := float64(after.HeapAlloc-before.HeapAlloc) / (1 << 20)
	t.Logf("%d rows resident in %.1f MiB (budget %d MiB)", index.Len(), residentMiB, budgetMiB)
	require.Equal(t, rows, index.Len())
	require.Lessf(t, residentMiB, float64(budgetMiB),
		"index grew to %.1f MiB; the 512 MiB container must hold two of these during a refresh", residentMiB)

	// Keep the index alive so the measurement cannot be collected early.
	_, ok := index.Lookup("tt0000001")
	require.True(t, ok)
}

// syntheticRatings streams a deterministic export-shaped dataset without holding
// it in memory, so the budget test measures the index and not its input.
func syntheticRatings(rows int) io.Reader {
	reader, writer := io.Pipe()
	go func() {
		buffered := make([]byte, 0, 64)
		_, _ = io.WriteString(writer, "tconst\taverageRating\tnumVotes\n")
		for row := 1; row <= rows; row++ {
			buffered = buffered[:0]
			buffered = append(buffered, formatTitleID(uint32(row))...)
			buffered = append(buffered, '\t')
			buffered = append(buffered, formatScaledRating(uint8(10+row%91))...)
			buffered = append(buffered, '\t')
			buffered = append(buffered, fmt.Sprintf("%d\n", row*7%3233728)...)
			if _, err := writer.Write(buffered); err != nil {
				break
			}
		}
		_ = writer.Close()
	}()
	return reader
}

func BenchmarkIndexLookup(b *testing.B) {
	index, err := ParseRatings(syntheticRatings(1_713_579))
	if err != nil {
		b.Fatal(err)
	}
	ids := []string{"tt0000001", "tt0500000", "tt1000000", "tt1500000", "tt1713579"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		if _, ok := index.Lookup(ids[i%len(ids)]); !ok {
			b.Fatal("expected hit")
		}
	}
}

func BenchmarkParseRatings(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ParseRatings(syntheticRatings(200_000)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseRatingsUnordered(b *testing.B) {
	raw := "tt10000000\t7.0\t1\ntt0000002\t8.0\t2\ntt0000001\t9.0\t3\n"
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := ParseRatingsWithStats(strings.NewReader(raw)); err != nil {
			b.Fatal(err)
		}
	}
}

func TestManagerRetainsPreviousSnapshotWhenRefreshFails(t *testing.T) {
	manager := NewManager(0)
	good := SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{Generation: 1, Body: body("tt0000001\t7.0\t10\n")}, nil
	})
	require.NoError(t, manager.Refresh(context.Background(), good))

	failed := SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{}, errors.New("storage unavailable")
	})
	err := manager.Refresh(context.Background(), failed)

	require.Error(t, err)
	rating, ok := manager.Lookup("tt0000001")
	require.True(t, ok)
	require.Equal(t, "7.0", rating.Rating)
}

func TestManagerSkipsUnchangedGeneration(t *testing.T) {
	manager := NewManager(0)
	calls := 0
	source := SourceFunc(func(_ context.Context, currentGeneration int64) (Snapshot, error) {
		calls++
		if currentGeneration == 7 {
			return Snapshot{Generation: 7, Unchanged: true}, nil
		}
		return Snapshot{Generation: 7, Body: body("tt0000001\t7.0\t10\n")}, nil
	})

	require.NoError(t, manager.Refresh(context.Background(), source))
	require.NoError(t, manager.Refresh(context.Background(), source))
	require.Equal(t, 2, calls)
	require.Equal(t, int64(7), manager.Generation())
}

// A leaked GCS stream would accumulate across refreshes on a long-lived instance.
func TestManagerClosesSnapshotBody(t *testing.T) {
	tracked := &trackedBody{Reader: strings.NewReader("tt0000001\t7.0\t10\n")}
	manager := NewManager(0)

	require.NoError(t, manager.Refresh(context.Background(), SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{Generation: 3, Body: tracked}, nil
	})))

	require.True(t, tracked.closed, "Refresh must close the snapshot body")
}

func TestManagerClosesSnapshotBodyWhenParseFails(t *testing.T) {
	tracked := &trackedBody{Reader: strings.NewReader("tt0000001\t7.0\t10\n"), readErr: errors.New("truncated stream")}
	manager := NewManager(0)

	err := manager.Refresh(context.Background(), SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{Generation: 3, Body: tracked}, nil
	}))

	require.ErrorContains(t, err, "truncated stream")
	require.True(t, tracked.closed)
}

func TestManagerRetainsPreviousSnapshotBelowRowFloor(t *testing.T) {
	manager := NewManager(2)
	require.NoError(t, manager.Refresh(context.Background(), SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{Generation: 1, Body: body("tt0000001\t7.0\t10\ntt0000002\t8.0\t20\n")}, nil
	})))

	err := manager.Refresh(context.Background(), SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{Generation: 2, Body: body("tconst\taverageRating\tnumVotes\n")}, nil
	}))

	require.ErrorIs(t, err, ErrSnapshotTooSmall)
	require.Equal(t, int64(1), manager.Generation())
	rating, ok := manager.Lookup("tt0000002")
	require.True(t, ok)
	require.Equal(t, "8.0", rating.Rating)
}

// With no index yet, a too-small snapshot must fail rather than make the service
// ready with nothing to serve.
func TestManagerRejectsInitialSnapshotBelowRowFloor(t *testing.T) {
	manager := NewManager(1)

	err := manager.Refresh(context.Background(), SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{Generation: 1, Body: body("")}, nil
	}))

	require.ErrorIs(t, err, ErrSnapshotTooSmall)
	require.Zero(t, manager.Generation())
}

func TestManagerStatsDescribeLiveIndex(t *testing.T) {
	require.Zero(t, NewManager(0).Stats())
	stats := loadedManager(t, "tt0000002\t8.0\t20\nbroken\ntt0000001\t7.0\t10\n").Stats()
	require.Equal(t, 2, stats.CompactRows)
	require.Equal(t, 1, stats.SkippedRows)
	require.True(t, stats.SortApplied)
}

func TestManagerRejectsSnapshotWithoutBody(t *testing.T) {
	manager := NewManager(0)
	err := manager.Refresh(context.Background(), SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{Generation: 1}, nil
	}))
	require.ErrorContains(t, err, "no body")
}

type trackedBody struct {
	*strings.Reader
	readErr error
	closed  bool
}

func (t *trackedBody) Read(p []byte) (int, error) {
	if t.readErr != nil {
		return 0, t.readErr
	}
	return t.Reader.Read(p)
}

func (t *trackedBody) Close() error {
	t.closed = true
	return nil
}

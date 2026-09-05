package imdb

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
)

var ErrRatingNotFound = errors.New("imdb rating not found")

// expectedRatingRows pre-sizes the entry slice. The IMDb ratings export held
// 1,713,579 rows when this was written; sizing above that avoids the doubling
// growth that would transiently hold both the old and new backing arrays.
const expectedRatingRows = 1_900_000

// canonicalTitleIDDigits is the width IMDb zero-pads short identifiers to, so
// title 1 is published as "tt0000001".
const canonicalTitleIDDigits = 7

type Rating struct {
	ID     string `json:"Id"`
	Rating string `json:"Rating"`
	Votes  string `json:"Votes"`
}

// entry is one ratings row in compact form: the numeric part of the tt
// identifier, the vote count, and the average rating scaled by ten so that 8.7
// is stored as 87. That is 9 bytes of payload in a 12-byte struct, against
// roughly 105 bytes for the equivalent map[string]Rating bucket.
type entry struct {
	id     uint32
	votes  uint32
	rating uint8
}

// Index answers rating lookups from a sorted slice of compact entries.
//
// Rows that cannot be represented compactly, which no row in the published
// dataset currently is, are kept verbatim in overflow so that malformed or
// future out-of-range input degrades into extra memory rather than into a wrong
// or missing answer.
type Index struct {
	entries  []entry
	overflow map[string]Rating
}

func ParseRatings(reader io.Reader) (*Index, error) {
	index := &Index{entries: make([]entry, 0, expectedRatingRows)}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		// Deliberately operating on Bytes rather than Text: Text would allocate
		// a string for every one of ~1.7M lines purely to throw it away.
		id, rating, votes, ok := splitRatingRow(scanner.Bytes())
		if !ok || bytes.Equal(id, []byte("tconst")) {
			continue
		}
		index.add(id, rating, votes)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan IMDb ratings: %w", err)
	}
	slices.SortFunc(index.entries, func(a, b entry) int { return cmp.Compare(a.id, b.id) })
	return index, nil
}

// splitRatingRow splits a tab-separated row into exactly three fields.
func splitRatingRow(row []byte) (id, rating, votes []byte, ok bool) {
	first := bytes.IndexByte(row, '\t')
	if first < 0 {
		return nil, nil, nil, false
	}
	remainder := row[first+1:]
	second := bytes.IndexByte(remainder, '\t')
	if second < 0 {
		return nil, nil, nil, false
	}
	votes = remainder[second+1:]
	if bytes.IndexByte(votes, '\t') >= 0 {
		return nil, nil, nil, false
	}
	return row[:first], remainder[:second], votes, true
}

func (i *Index) add(id, rating, votes []byte) {
	numericID, idOK := parseCanonicalTitleID(id)
	scaledRating, ratingOK := parseScaledRating(rating)
	voteCount, votesOK := parseUint32(votes)
	if idOK && ratingOK && votesOK {
		i.entries = append(i.entries, entry{id: numericID, votes: voteCount, rating: scaledRating})
		return
	}
	if i.overflow == nil {
		i.overflow = make(map[string]Rating)
	}
	i.overflow[string(id)] = Rating{ID: string(id), Rating: string(rating), Votes: string(votes)}
}

func (i *Index) Lookup(id string) (Rating, bool) {
	if i == nil {
		return Rating{}, false
	}
	if rating, ok := i.overflow[id]; ok {
		return rating, true
	}
	numericID, ok := parseCanonicalTitleID([]byte(id))
	if !ok {
		return Rating{}, false
	}
	position, found := slices.BinarySearchFunc(i.entries, numericID, func(candidate entry, target uint32) int {
		return cmp.Compare(candidate.id, target)
	})
	if !found {
		return Rating{}, false
	}
	return i.entries[position].toRating(), true
}

// Len reports how many rows the index holds. It exists for tests and metrics.
func (i *Index) Len() int {
	if i == nil {
		return 0
	}
	return len(i.entries) + len(i.overflow)
}

func (e entry) toRating() Rating {
	return Rating{ID: formatTitleID(e.id), Rating: formatScaledRating(e.rating), Votes: strconv.FormatUint(uint64(e.votes), 10)}
}

// parseCanonicalTitleID accepts only the exact form IMDb publishes, so that
// compacting identifiers into integers cannot make lookups more permissive than
// the map-based index it replaces. "tt0133093" resolves; "tt133093" does not.
func parseCanonicalTitleID(id []byte) (uint32, bool) {
	if len(id) < 2 || id[0] != 't' || id[1] != 't' {
		return 0, false
	}
	digits := id[2:]
	if len(digits) < canonicalTitleIDDigits {
		return 0, false
	}
	if len(digits) > canonicalTitleIDDigits && digits[0] == '0' {
		return 0, false
	}
	var value uint64
	for _, character := range digits {
		if character < '0' || character > '9' {
			return 0, false
		}
		value = value*10 + uint64(character-'0')
		if value > math.MaxUint32 {
			return 0, false
		}
	}
	return uint32(value), true
}

func formatTitleID(id uint32) string {
	digits := strconv.FormatUint(uint64(id), 10)
	if len(digits) < canonicalTitleIDDigits {
		return "tt" + strings.Repeat("0", canonicalTitleIDDigits-len(digits)) + digits
	}
	return "tt" + digits
}

// parseScaledRating accepts the published one-decimal form and scales it by ten.
func parseScaledRating(rating []byte) (uint8, bool) {
	point := bytes.IndexByte(rating, '.')
	if point < 1 || point != len(rating)-2 {
		return 0, false
	}
	whole, ok := parseUint32(rating[:point])
	if !ok {
		return 0, false
	}
	fraction := rating[len(rating)-1]
	if fraction < '0' || fraction > '9' {
		return 0, false
	}
	scaled := uint64(whole)*10 + uint64(fraction-'0')
	if scaled > math.MaxUint8 {
		return 0, false
	}
	return uint8(scaled), true
}

func formatScaledRating(rating uint8) string {
	return strconv.FormatUint(uint64(rating/10), 10) + "." + string(rune('0'+rating%10))
}

func parseUint32(value []byte) (uint32, bool) {
	if len(value) == 0 {
		return 0, false
	}
	var parsed uint64
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, false
		}
		parsed = parsed*10 + uint64(character-'0')
		if parsed > math.MaxUint32 {
			return 0, false
		}
	}
	return uint32(parsed), true
}

// Snapshot carries an undecoded ratings stream. Body is nil when Unchanged is
// set; otherwise the caller owns it and must close it.
type Snapshot struct {
	Body       io.ReadCloser
	Generation int64
	Unchanged  bool
}

type Source interface {
	Open(context.Context, int64) (Snapshot, error)
}

type SourceFunc func(context.Context, int64) (Snapshot, error)

func (f SourceFunc) Open(ctx context.Context, generation int64) (Snapshot, error) {
	return f(ctx, generation)
}

type state struct {
	generation int64
	index      *Index
}

type Manager struct {
	current atomic.Pointer[state]
}

func NewManager() *Manager { return &Manager{} }

func (m *Manager) Refresh(ctx context.Context, source Source) error {
	snapshot, err := source.Open(ctx, m.Generation())
	if err != nil {
		return err
	}
	if snapshot.Body != nil {
		defer func() { _ = snapshot.Body.Close() }()
	}
	if snapshot.Unchanged {
		return nil
	}
	if snapshot.Body == nil {
		return errors.New("ratings snapshot has no body")
	}
	index, err := ParseRatings(snapshot.Body)
	if err != nil {
		return err
	}
	m.current.Store(&state{generation: snapshot.Generation, index: index})
	return nil
}

func (m *Manager) Lookup(id string) (Rating, bool) {
	current := m.current.Load()
	if current == nil {
		return Rating{}, false
	}
	return current.index.Lookup(id)
}

func (m *Manager) Generation() int64 {
	current := m.current.Load()
	if current == nil {
		return 0
	}
	return current.generation
}

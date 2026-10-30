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

// ErrSnapshotTooSmall rejects a ratings export that parsed but holds far fewer
// rows than the published dataset. A valid gzip of a header-only, truncated, or
// error-page export would otherwise replace a good index and turn every lookup
// into a 404 while the service still reports ready.
var ErrSnapshotTooSmall = errors.New("imdb ratings snapshot has too few rows")

// MinSnapshotRows is the floor production enforces before publishing or
// serving a snapshot. The export held 1,713,579 rows when this was written; the
// floor sits well below that so normal churn never trips it.
const MinSnapshotRows = 1_000_000

// expectedRatingRows pre-sizes the entry slice. The IMDb ratings export held
// 1,713,579 rows when this was written; sizing above that avoids the doubling
// growth that would transiently hold both the old and new backing arrays.
const expectedRatingRows = 1_900_000

// The published export is about 30 MiB. Bound expanded bytes as well as archive
// bytes: a small gzip can otherwise allocate an arbitrarily large index.
const maxRatingsExportSize = 64 << 20

const (
	maxRatingRows    = 2_500_000
	maxOverflowRows  = 4096
	maxOverflowBytes = 4 << 20
)

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
	entries       []entry
	overflow      map[string]Rating
	overflowBytes int
}

type ParseStats struct {
	CompactRows  int
	InputOrdered bool
	OverflowRows int
	SkippedRows  int
	SortApplied  bool
}

// Rows reports how many rows the parsed index holds.
func (s ParseStats) Rows() int { return s.CompactRows + s.OverflowRows }

// CheckMinRows reports ErrSnapshotTooSmall when stats fall below minRows.
func CheckMinRows(stats ParseStats, minRows int) error {
	if stats.Rows() < minRows {
		return fmt.Errorf("%w: parsed %d rows, need at least %d", ErrSnapshotTooSmall, stats.Rows(), minRows)
	}
	return nil
}

func ParseRatings(reader io.Reader) (*Index, error) {
	index, _, err := ParseRatingsWithStats(reader)
	return index, err
}

func ParseRatingsWithStats(reader io.Reader) (*Index, ParseStats, error) {
	index := &Index{entries: make([]entry, 0, expectedRatingRows)}
	stats := ParseStats{InputOrdered: true}
	var previousID uint32
	hasPrevious := false
	limited := &io.LimitedReader{R: reader, N: maxRatingsExportSize + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		// Deliberately operating on Bytes rather than Text: Text would allocate
		// a string for every one of ~1.7M lines purely to throw it away.
		id, rating, votes, ok := splitRatingRow(scanner.Bytes())
		if !ok {
			stats.SkippedRows++
			continue
		}
		if bytes.Equal(id, []byte("tconst")) {
			continue
		}
		if !validTitleID(id) || len(rating) == 0 || len(votes) == 0 {
			stats.SkippedRows++
			continue
		}
		beforeCompact := len(index.entries)
		if err := index.add(id, rating, votes); err != nil {
			return nil, stats, err
		}
		if len(index.entries) > beforeCompact {
			currentID := index.entries[len(index.entries)-1].id
			if hasPrevious && currentID < previousID {
				stats.InputOrdered = false
			}
			previousID = currentID
			hasPrevious = true
			stats.CompactRows++
		} else {
			stats.OverflowRows++
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, stats, fmt.Errorf("scan IMDb ratings: %w", err)
	}
	if limited.N == 0 {
		return nil, stats, fmt.Errorf("ratings export exceeds %d bytes", maxRatingsExportSize)
	}
	if !stats.InputOrdered {
		slices.SortFunc(index.entries, func(a, b entry) int { return cmp.Compare(a.id, b.id) })
		stats.SortApplied = true
	}
	for position := 1; position < len(index.entries); position++ {
		if index.entries[position-1].id == index.entries[position].id {
			return nil, stats, fmt.Errorf("duplicate IMDb title %s", formatTitleID(index.entries[position].id))
		}
	}
	for id := range index.overflow {
		if numericID, ok := parseCanonicalTitleID([]byte(id)); ok {
			if _, found := slices.BinarySearchFunc(index.entries, numericID, func(candidate entry, target uint32) int {
				return cmp.Compare(candidate.id, target)
			}); found {
				return nil, stats, fmt.Errorf("duplicate IMDb title %s", id)
			}
		}
	}
	return index, stats, nil
}

// Check cancellation at each buffered read, including CPU-only decompression of
// an already downloaded archive. Network-backed readers also receive the same
// context through Source.Open so cancellation interrupts blocked storage I/O.
type ratingsContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r ratingsContextReader) Read(destination []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(destination)
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

func (i *Index) add(id, rating, votes []byte) error {
	if _, exists := i.overflow[string(id)]; exists {
		return fmt.Errorf("duplicate IMDb title %s", id)
	}
	numericID, idOK := parseCanonicalTitleID(id)
	scaledRating, ratingOK := parseScaledRating(rating)
	voteCount, votesOK := parseUint32(votes)
	if idOK && ratingOK && votesOK {
		if i.Len() >= maxRatingRows {
			return fmt.Errorf("ratings index exceeds %d rows", maxRatingRows)
		}
		if len(i.entries) == cap(i.entries) {
			// Cap backing-array growth too; append's default growth can retain
			// more entries than the admitted row budget.
			entries := make([]entry, len(i.entries), min(maxRatingRows, cap(i.entries)+cap(i.entries)/4))
			copy(entries, i.entries)
			i.entries = entries
		}
		i.entries = append(i.entries, entry{id: numericID, votes: voteCount, rating: scaledRating})
		return nil
	}
	bytes := len(id) + len(rating) + len(votes)
	if len(i.overflow) >= maxOverflowRows || i.overflowBytes+bytes > maxOverflowBytes {
		return errors.New("ratings overflow budget exceeded")
	}
	if i.Len() >= maxRatingRows {
		return fmt.Errorf("ratings index exceeds %d rows", maxRatingRows)
	}
	if i.overflow == nil {
		i.overflow = make(map[string]Rating)
	}
	i.overflow[string(id)] = Rating{ID: string(id), Rating: string(rating), Votes: string(votes)}
	i.overflowBytes += bytes
	return nil
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

// validTitleID checks the published spelling without restricting the numeric
// range, so future identifiers beyond uint32 still use the overflow fallback.
func validTitleID(id []byte) bool {
	if len(id) < 2+canonicalTitleIDDigits || id[0] != 't' || id[1] != 't' {
		return false
	}
	if len(id) > 2+canonicalTitleIDDigits && id[2] == '0' {
		return false
	}
	for _, digit := range id[2:] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
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

// SnapshotFormat says how a Snapshot body is encoded.
type SnapshotFormat int

const (
	// FormatExport is the decompressed IMDb tab-separated export.
	FormatExport SnapshotFormat = iota
	// FormatIndex is the binary layout written by EncodeIndex.
	FormatIndex
)

// Snapshot carries an undecoded ratings stream. Body is nil when Unchanged is
// set; otherwise the caller owns it and must close it.
type Snapshot struct {
	Body       io.ReadCloser
	Format     SnapshotFormat
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
	stats      ParseStats
}

type Manager struct {
	current atomic.Pointer[state]
	minRows int
}

// NewManager returns a Manager that refuses to swap in a snapshot holding fewer
// than minRows rows, keeping the current index instead.
func NewManager(minRows int) *Manager { return &Manager{minRows: minRows} }

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
	body := ratingsContextReader{ctx: ctx, reader: snapshot.Body}
	var (
		index *Index
		stats ParseStats
	)
	switch snapshot.Format {
	case FormatExport:
		index, stats, err = ParseRatingsWithStats(body)
	case FormatIndex:
		index, stats, err = DecodeIndex(body)
	default:
		err = fmt.Errorf("unknown ratings snapshot format %d", snapshot.Format)
	}
	if err != nil {
		return err
	}
	if err := CheckMinRows(stats, m.minRows); err != nil {
		return fmt.Errorf("ratings generation %d: %w", snapshot.Generation, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.current.Store(&state{generation: snapshot.Generation, index: index, stats: stats})
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

// Stats reports how the live index was parsed.
func (m *Manager) Stats() ParseStats {
	current := m.current.Load()
	if current == nil {
		return ParseStats{}
	}
	return current.stats
}

// Len reports how many rows the live index holds.
func (m *Manager) Len() int {
	current := m.current.Load()
	if current == nil {
		return 0
	}
	return current.index.Len()
}

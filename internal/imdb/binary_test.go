package imdb

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"hash/crc32"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const mixedRatings = "tconst\taverageRating\tnumVotes\n" +
	"tt0000001\t5.6\t2100\n" +
	"tt0133093\t8.7\t2100000\n" +
	"tt45115568\t10.0\t3233728\n" +
	"tt99999999999\t7.5\t20\n" + // identifier exceeds uint32
	"tt0000002\t7.25\t30\n" + // two decimal places
	"tt0000003\t6.1\t99999999999\n" // vote count exceeds uint32

func encoded(t *testing.T, raw string) []byte {
	t.Helper()
	index, err := ParseRatings(strings.NewReader(raw))
	require.NoError(t, err)
	var buffer bytes.Buffer
	require.NoError(t, EncodeIndex(&buffer, index))
	return buffer.Bytes()
}

// resign recomputes the trailing checksum so a test can corrupt the body and
// reach the structural check behind it.
func resign(data []byte) []byte {
	body := data[:len(data)-4]
	return binary.LittleEndian.AppendUint32(bytes.Clone(body), crc32.ChecksumIEEE(body))
}

func TestIndexRoundTripsCompactAndOverflowRows(t *testing.T) {
	parsed, err := ParseRatings(strings.NewReader(mixedRatings))
	require.NoError(t, err)

	decoded, stats, err := DecodeIndex(bytes.NewReader(encoded(t, mixedRatings)))

	require.NoError(t, err)
	require.Equal(t, ParseStats{CompactRows: 3, InputOrdered: true, OverflowRows: 3}, stats)
	require.Equal(t, parsed.entries, decoded.entries)
	require.Equal(t, parsed.overflow, decoded.overflow)
	require.Equal(t, parsed.overflowBytes, decoded.overflowBytes)
	for _, id := range []string{"tt0000001", "tt0133093", "tt45115568", "tt99999999999", "tt0000002", "tt0000003"} {
		want, _ := parsed.Lookup(id)
		got, ok := decoded.Lookup(id)
		require.Truef(t, ok, "expected %s to resolve", id)
		require.Equal(t, want, got)
	}
	_, ok := decoded.Lookup("tt133093")
	require.False(t, ok, "decoding must keep the canonical-identifier contract")
}

func TestMarshalIndexProducesDecodableBytes(t *testing.T) {
	index, err := ParseRatings(strings.NewReader(mixedRatings))
	require.NoError(t, err)

	data, err := MarshalIndex(index)

	require.NoError(t, err)
	require.Equal(t, encoded(t, mixedRatings), data)
	_, err = MarshalIndex(nil)
	require.ErrorContains(t, err, "index is nil")
}

func TestEncodeIndexIsDeterministic(t *testing.T) {
	require.Equal(t, encoded(t, mixedRatings), encoded(t, mixedRatings))
}

func TestDecodeIndexRoundTripsEmptyIndex(t *testing.T) {
	decoded, stats, err := DecodeIndex(bytes.NewReader(encoded(t, "")))

	require.NoError(t, err)
	require.Equal(t, 0, decoded.Len())
	require.Equal(t, 0, stats.Rows())
}

func TestDecodeIndexRejectsCorruption(t *testing.T) {
	valid := encoded(t, "tt0000001\t5.6\t2100\ntt0000002\t6.0\t10\ntt0000003\t7.25\t1\n")
	headerCounts := func(compact, overflow uint32) []byte {
		data := bytes.Clone(valid)
		binary.LittleEndian.PutUint32(data[8:12], compact)
		binary.LittleEndian.PutUint32(data[12:16], overflow)
		return resign(data)
	}
	secondID := indexHeaderSize + indexEntrySize
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"empty", nil, "read header"},
		{"wrong magic", resign(append([]byte("NOTANIDX"), valid[8:]...)), "unrecognised header"},
		{"too many rows", headerCounts(maxRatingRows+1, 0), "exceed the limits"},
		{"too many overflow rows", headerCounts(2, maxOverflowRows+1), "exceed the limits"},
		{"truncated rows", valid[:secondID+3], "read rows"},
		{"truncated overflow", valid[:len(valid)-6], "read overflow field"},
		{"missing checksum", valid[:len(valid)-4], "read checksum"},
		{"checksum mismatch", append(bytes.Clone(valid[:len(valid)-1]), valid[len(valid)-1]^0xff), "checksum mismatch"},
		{"flipped row byte", func() []byte {
			data := bytes.Clone(valid)
			data[indexHeaderSize+4] ^= 0xff
			return data
		}(), "checksum mismatch"},
		{"trailing bytes", append(bytes.Clone(valid), 0), "trailing bytes"},
		{"duplicate compact id", func() []byte {
			data := bytes.Clone(valid)
			copy(data[secondID:secondID+4], data[indexHeaderSize:indexHeaderSize+4])
			return resign(data)
		}(), "out of order or duplicated"},
		{"descending compact id", func() []byte {
			data := bytes.Clone(valid)
			binary.LittleEndian.PutUint32(data[indexHeaderSize:], 9)
			return resign(data)
		}(), "out of order or duplicated"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			index, _, err := DecodeIndex(bytes.NewReader(test.data))

			require.ErrorIs(t, err, ErrInvalidIndex)
			require.ErrorContains(t, err, test.want)
			require.Nil(t, index)
		})
	}
}

func TestDecodeIndexRejectsOverflowRowsTheParserWouldReject(t *testing.T) {
	build := func(compact []entry, overflow ...[3]string) []byte {
		var data []byte
		data = append(data, indexMagic[:]...)
		data = binary.LittleEndian.AppendUint32(data, uint32(len(compact)))
		data = binary.LittleEndian.AppendUint32(data, uint32(len(overflow)))
		for _, e := range compact {
			data = binary.LittleEndian.AppendUint32(data, e.id)
			data = binary.LittleEndian.AppendUint32(data, e.votes)
			data = append(data, e.rating)
		}
		for _, row := range overflow {
			for _, field := range row {
				data = binary.LittleEndian.AppendUint16(data, uint16(len(field)))
				data = append(data, field...)
			}
		}
		return binary.LittleEndian.AppendUint32(data, crc32.ChecksumIEEE(data))
	}
	// Each row fits a uint16 field; together they pass maxOverflowBytes.
	var oversized [][3]string
	for row := 1; row <= maxOverflowBytes/60000+1; row++ {
		oversized = append(oversized, [3]string{formatTitleID(uint32(row)), strings.Repeat("9", 60000), "1"})
	}
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"non-canonical id", build(nil, [3]string{"tt1", "7.25", "1"}), "malformed overflow row"},
		{"empty rating", build(nil, [3]string{"tt0000001", "", "1"}), "malformed overflow row"},
		{"shadows compact row", build([]entry{{id: 1, votes: 1, rating: 70}}, [3]string{"tt0000001", "7.25", "1"}), "duplicate title"},
		{"repeated overflow row", build(nil, [3]string{"tt0000001", "7.25", "1"}, [3]string{"tt0000001", "7.25", "1"}), "duplicate title"},
		{"overflow budget", build(nil, oversized...), "overflow rows exceed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := DecodeIndex(bytes.NewReader(test.data))

			require.ErrorIs(t, err, ErrInvalidIndex)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestManagerLoadsBinaryIndexSnapshot(t *testing.T) {
	manager := NewManager(3)
	source := SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{Generation: 7, Format: FormatIndex, Body: io.NopCloser(bytes.NewReader(encoded(t, mixedRatings)))}, nil
	})

	require.NoError(t, manager.Refresh(context.Background(), source))

	require.Equal(t, int64(7), manager.Generation())
	require.Equal(t, 6, manager.Len())
	got, ok := manager.Lookup("tt0133093")
	require.True(t, ok)
	require.Equal(t, Rating{ID: "tt0133093", Rating: "8.7", Votes: "2100000"}, got)
}

func TestManagerRetainsSnapshotWhenBinaryIndexIsCorrupt(t *testing.T) {
	manager := NewManager(0)
	require.NoError(t, manager.Refresh(context.Background(), SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{Generation: 1, Body: body("tt0000001\t7.0\t10\n")}, nil
	})))
	corrupt := encoded(t, mixedRatings)
	corrupt[len(corrupt)-1] ^= 0xff

	err := manager.Refresh(context.Background(), SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{Generation: 2, Format: FormatIndex, Body: io.NopCloser(bytes.NewReader(corrupt))}, nil
	}))

	require.ErrorIs(t, err, ErrInvalidIndex)
	require.Equal(t, int64(1), manager.Generation())
}

func TestManagerRejectsUnknownSnapshotFormat(t *testing.T) {
	err := NewManager(0).Refresh(context.Background(), SourceFunc(func(context.Context, int64) (Snapshot, error) {
		return Snapshot{Generation: 1, Format: SnapshotFormat(99), Body: body("")}, nil
	}))

	require.ErrorContains(t, err, "unknown ratings snapshot format")
}

// The two benchmarks below compare a cold start's index load from each
// published object, at production size, with the input already in memory so
// only decoding is measured.
func BenchmarkLoadExportSnapshot(b *testing.B) {
	var archive bytes.Buffer
	writer := gzip.NewWriter(&archive)
	if _, err := io.Copy(writer, syntheticRatings(1_713_579)); err != nil {
		b.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		reader, err := gzip.NewReader(bytes.NewReader(archive.Bytes()))
		if err != nil {
			b.Fatal(err)
		}
		if _, err := ParseRatings(reader); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(archive.Len())/(1<<20), "object-MiB")
}

func BenchmarkLoadIndexSnapshot(b *testing.B) {
	index, err := ParseRatings(syntheticRatings(1_713_579))
	if err != nil {
		b.Fatal(err)
	}
	var data bytes.Buffer
	if err := EncodeIndex(&data, index); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := DecodeIndex(bytes.NewReader(data.Bytes())); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(data.Len())/(1<<20), "object-MiB")
}

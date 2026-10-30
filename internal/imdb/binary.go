package imdb

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"maps"
	"math"
	"slices"
)

// The binary index is the parsed Index written out so a cold-starting service
// can load it without tokenising, validating, and sorting ~1.7M text rows. The
// download job produces it from an export it has already parsed and accepted.
//
// Layout, little-endian:
//
//	magic           8 bytes
//	compact count   uint32
//	overflow count  uint32
//	compact rows    count × (id uint32, votes uint32, rating uint8), ascending id
//	overflow rows   count × 3 × (uint16 length, bytes): id, rating, votes
//	checksum        uint32 CRC-32 (IEEE) of everything before it
var indexMagic = [8]byte{'M', 'T', 'I', 'M', 'D', 'B', 0, 1}

const (
	indexHeaderSize = len(indexMagic) + 8
	indexEntrySize  = 9
	// indexChunkRows bounds the decode buffer; reading row by row through
	// io.ReadFull would cost a call per row.
	indexChunkRows = 4096
)

var ErrInvalidIndex = errors.New("invalid IMDb binary index")

// EncodeIndex writes index in the binary layout.
func EncodeIndex(writer io.Writer, index *Index) error {
	if index == nil {
		return errors.New("encode IMDb index: index is nil")
	}
	if len(index.entries) > maxRatingRows || len(index.overflow) > maxOverflowRows {
		return fmt.Errorf("encode IMDb index: %d rows exceed the format limits", index.Len())
	}
	checksum := crc32.NewIEEE()
	buffered := bufio.NewWriter(io.MultiWriter(writer, checksum))
	header := make([]byte, 0, indexHeaderSize)
	header = append(header, indexMagic[:]...)
	header = binary.LittleEndian.AppendUint32(header, uint32(len(index.entries)))
	header = binary.LittleEndian.AppendUint32(header, uint32(len(index.overflow)))
	if _, err := buffered.Write(header); err != nil {
		return fmt.Errorf("encode IMDb index: %w", err)
	}
	var record [indexEntrySize]byte
	for _, entry := range index.entries {
		binary.LittleEndian.PutUint32(record[0:4], entry.id)
		binary.LittleEndian.PutUint32(record[4:8], entry.votes)
		record[8] = entry.rating
		if _, err := buffered.Write(record[:]); err != nil {
			return fmt.Errorf("encode IMDb index: %w", err)
		}
	}
	// Sorted so the same index always encodes to the same bytes.
	for _, id := range slices.Sorted(maps.Keys(index.overflow)) {
		rating := index.overflow[id]
		for _, field := range []string{rating.ID, rating.Rating, rating.Votes} {
			if len(field) > math.MaxUint16 {
				return fmt.Errorf("encode IMDb index: overflow field of %d bytes", len(field))
			}
			var length [2]byte
			binary.LittleEndian.PutUint16(length[:], uint16(len(field)))
			if _, err := buffered.Write(length[:]); err != nil {
				return fmt.Errorf("encode IMDb index: %w", err)
			}
			if _, err := buffered.WriteString(field); err != nil {
				return fmt.Errorf("encode IMDb index: %w", err)
			}
		}
	}
	if err := buffered.Flush(); err != nil {
		return fmt.Errorf("encode IMDb index: %w", err)
	}
	if _, err := writer.Write(binary.LittleEndian.AppendUint32(nil, checksum.Sum32())); err != nil {
		return fmt.Errorf("encode IMDb index: %w", err)
	}
	return nil
}

// MarshalIndex encodes index and decodes the result back, so the download job
// publishes only bytes the service is known to accept.
func MarshalIndex(index *Index) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.Grow(indexHeaderSize + index.Len()*indexEntrySize + 4)
	if err := EncodeIndex(&buffer, index); err != nil {
		return nil, err
	}
	decoded, _, err := DecodeIndex(bytes.NewReader(buffer.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("verify encoded IMDb index: %w", err)
	}
	if decoded.Len() != index.Len() {
		return nil, fmt.Errorf("verify encoded IMDb index: %d rows decoded from %d", decoded.Len(), index.Len())
	}
	return buffer.Bytes(), nil
}

// DecodeIndex reads the binary layout and re-checks every invariant the text
// parser establishes, so a corrupt or hostile object cannot produce an index
// that answers differently from parsing the export.
func DecodeIndex(reader io.Reader) (*Index, ParseStats, error) {
	checksum := crc32.NewIEEE()
	buffered := bufio.NewReaderSize(reader, 64*1024)
	body := io.TeeReader(buffered, checksum)

	var header [indexHeaderSize]byte
	if _, err := io.ReadFull(body, header[:]); err != nil {
		return nil, ParseStats{}, invalidIndex("read header: %v", err)
	}
	if !bytes.Equal(header[:len(indexMagic)], indexMagic[:]) {
		return nil, ParseStats{}, invalidIndex("unrecognised header")
	}
	compactRows := int(binary.LittleEndian.Uint32(header[8:12]))
	overflowRows := int(binary.LittleEndian.Uint32(header[12:16]))
	if compactRows > maxRatingRows || overflowRows > maxOverflowRows || compactRows+overflowRows > maxRatingRows {
		return nil, ParseStats{}, invalidIndex("%d compact and %d overflow rows exceed the limits", compactRows, overflowRows)
	}

	// Sized exactly: unlike the text parser, the row count is known up front.
	index := &Index{entries: make([]entry, compactRows)}
	chunk := make([]byte, indexChunkRows*indexEntrySize)
	for start := 0; start < compactRows; start += indexChunkRows {
		rows := min(indexChunkRows, compactRows-start)
		if _, err := io.ReadFull(body, chunk[:rows*indexEntrySize]); err != nil {
			return nil, ParseStats{}, invalidIndex("read rows: %v", err)
		}
		for row := range rows {
			record := chunk[row*indexEntrySize:]
			index.entries[start+row] = entry{
				id:     binary.LittleEndian.Uint32(record[0:4]),
				votes:  binary.LittleEndian.Uint32(record[4:8]),
				rating: record[8],
			}
		}
	}
	for position := 1; position < compactRows; position++ {
		if index.entries[position-1].id >= index.entries[position].id {
			return nil, ParseStats{}, invalidIndex("title %s is out of order or duplicated", formatTitleID(index.entries[position].id))
		}
	}

	for range overflowRows {
		var fields [3][]byte
		for field := range fields {
			value, err := readIndexField(body)
			if err != nil {
				return nil, ParseStats{}, err
			}
			fields[field] = value
		}
		id, rating, votes := fields[0], fields[1], fields[2]
		if !validTitleID(id) || len(rating) == 0 || len(votes) == 0 {
			return nil, ParseStats{}, invalidIndex("malformed overflow row %q", id)
		}
		if numericID, ok := parseCanonicalTitleID(id); ok {
			if _, found := slices.BinarySearchFunc(index.entries, numericID, func(candidate entry, target uint32) int {
				return cmp.Compare(candidate.id, target)
			}); found {
				return nil, ParseStats{}, invalidIndex("duplicate title %s", id)
			}
		}
		if _, exists := index.overflow[string(id)]; exists {
			return nil, ParseStats{}, invalidIndex("duplicate title %s", id)
		}
		size := len(id) + len(rating) + len(votes)
		if index.overflowBytes+size > maxOverflowBytes {
			return nil, ParseStats{}, invalidIndex("overflow rows exceed %d bytes", maxOverflowBytes)
		}
		if index.overflow == nil {
			index.overflow = make(map[string]Rating, overflowRows)
		}
		index.overflow[string(id)] = Rating{ID: string(id), Rating: string(rating), Votes: string(votes)}
		index.overflowBytes += size
	}

	// The checksum is read past the tee so it does not hash itself.
	var trailer [4]byte
	if _, err := io.ReadFull(buffered, trailer[:]); err != nil {
		return nil, ParseStats{}, invalidIndex("read checksum: %v", err)
	}
	if binary.LittleEndian.Uint32(trailer[:]) != checksum.Sum32() {
		return nil, ParseStats{}, invalidIndex("checksum mismatch")
	}
	if _, err := buffered.ReadByte(); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, ParseStats{}, invalidIndex("read past checksum: %v", err)
		}
		return nil, ParseStats{}, invalidIndex("trailing bytes after checksum")
	}
	return index, ParseStats{CompactRows: compactRows, InputOrdered: true, OverflowRows: overflowRows}, nil
}

func readIndexField(reader io.Reader) ([]byte, error) {
	var length [2]byte
	if _, err := io.ReadFull(reader, length[:]); err != nil {
		return nil, invalidIndex("read overflow field: %v", err)
	}
	value := make([]byte, binary.LittleEndian.Uint16(length[:]))
	if _, err := io.ReadFull(reader, value); err != nil {
		return nil, invalidIndex("read overflow field: %v", err)
	}
	return value, nil
}

func invalidIndex(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidIndex, fmt.Sprintf(format, args...))
}

package catalog

import (
	"strings"
	"time"
)

type Collection string

const (
	CollectionLatest Collection = "latesttorrentsmovies"
	CollectionHDR10  Collection = "hdr10movies"
	CollectionDV     Collection = "dvmovies"
)

func (c Collection) Valid() bool {
	switch c {
	case CollectionLatest, CollectionHDR10, CollectionDV:
		return true
	default:
		return false
	}
}

// Accepts reports whether a release belongs in the collection by its own title.
// The HDR10 and DV collections are fed by Rutor text searches, which match
// anywhere in a title; requiring the format flag keeps a broadened or changed
// search from filling them with releases in a different format. HDR10+ counts
// as HDR10. The latest collection takes every release.
func (c Collection) Accepts(torrent Torrent) bool {
	switch c {
	case CollectionHDR10:
		return torrent.HDR10
	case CollectionDV:
		return torrent.DV
	default:
		return true
	}
}

type Torrent struct {
	Date         time.Time
	DetailsURL   string
	DV           bool
	FHD          bool
	HDR          bool
	HDR10        bool
	HDR10Plus    bool
	Hash         string
	K4           bool
	Leeches      int32
	Magnet       string
	MagnetHash   string
	Name         string
	OriginalName string
	RussianName  string
	Seeds        int32
	Size         float32
	Year         string
}

type Movie struct {
	BackdropPath  string    `firestore:"backdrop_path" json:"backdrop_path"`
	GenreIDs      []int32   `firestore:"genre_ids" json:"genre_ids"`
	ID            string    `firestore:"id" json:"id"`
	LastTimeFound time.Time `firestore:"lasttimefound" json:"lasttimefound"`
	OriginalTitle string    `firestore:"original_title" json:"original_title"`
	PosterPath    string    `firestore:"poster_path" json:"poster_path"`
	ReleaseDate   string    `firestore:"release_date" json:"release_date"`
	SearchAliases []string  `firestore:"-" json:"-"`
	SearchName    string    `firestore:"-" json:"-"`
	Title         string    `firestore:"title" json:"title"`
	VoteAverage   string    `firestore:"vote_average" json:"vote_average"`
	VoteCount     string    `firestore:"vote_count" json:"vote_count"`
	Year          string    `firestore:"Year" json:"Year"`
}

func GroupMovies(torrents []Torrent) []Movie {
	grouped := make(map[string]Movie)
	order := make([]string, 0, len(torrents))
	for _, torrent := range torrents {
		key := firstNonEmpty(torrent.Hash, torrent.MagnetHash, torrent.Name)
		if key == "" {
			continue
		}
		movie, exists := grouped[key]
		if !exists {
			aliases := distinctNonEmpty(torrent.OriginalName, torrent.RussianName)
			if len(aliases) == 0 {
				aliases = distinctNonEmpty(torrent.Name)
			}
			movie = Movie{
				SearchAliases: aliases,
				SearchName:    firstNonEmpty(aliases...),
				Year:          torrent.Year,
			}
			order = append(order, key)
		}
		if torrent.Date.After(movie.LastTimeFound) {
			movie.LastTimeFound = torrent.Date
		}
		grouped[key] = movie
	}

	movies := make([]Movie, 0, len(order))
	for _, key := range order {
		movies = append(movies, grouped[key])
	}
	return movies
}

func distinctNonEmpty(values ...string) []string {
	result := make([]string, 0, min(2, len(values)))
	seen := make(map[string]struct{}, 2)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
		if len(result) == 2 {
			break
		}
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

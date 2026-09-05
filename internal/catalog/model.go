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
			movie = Movie{
				SearchName: firstNonEmpty(torrent.OriginalName, torrent.RussianName, torrent.Name),
				Year:       torrent.Year,
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

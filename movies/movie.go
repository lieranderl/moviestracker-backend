package movies

import (

	"moviestracker/torrents"
	"github.com/lieranderl/go-tmdb"
)

type Short struct {
	tmdb.MovieShort 
	Year string
	Torrents []*torrents.Torrent
	Hash string
	Searchname string
	K4           bool
	FHD          bool
	HDR          bool
	DV           bool
}


func (m *Short) setQualityVector(){
	m.K4 = false
	m.FHD = false
	m.HDR = false
	m.DV  = false
	for _, t := range m.Torrents {
		if t.K4 {
			m.K4 = true
		}
		if t.FHD {
			m.FHD = true
		}
		if t.HDR {
			m.HDR = true
		}
		if t.DV {
			m.DV = true
		}
	}
}
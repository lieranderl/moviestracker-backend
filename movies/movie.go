package movies

import (
	"moviestracker/torrents"
	"time"

	"github.com/lieranderl/go-tmdb"
)

type Short struct {
	tmdb.MovieShort 
	Year 			string
	Torrents 		[]*torrents.Torrent
	Hash 			string
	Searchname 		string
	K4           	bool
	FHD         	bool
	HDR          	bool
	DV           	bool
	LastTimeFound 	time.Time
}


func (m *Short) updateMoviesAttribs(){
	m.setQualityVector()
	m.setLastimeFound()
}

func (m *Short) setQualityVector(){
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

func (m *Short) setLastimeFound(){
	for _, t := range m.Torrents {
		
		if t.Date == "" {
			t.Date = time.Now().String()
		}
		layout := "2006-01-02T15:04:05.000Z"
		timeformat, _ := time.Parse(layout, t.Date)

		if timeformat.After(m.LastTimeFound) {
			m.LastTimeFound = timeformat
		}
		
		
	}
}
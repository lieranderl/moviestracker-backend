package movies

import (
	"context"
	"log"
	// "math/rand"

	"cloud.google.com/go/firestore"
	"moviestracker/pipeline"
	"github.com/lieranderl/go-tmdb"
)


type TMDb struct{
	tmdb *tmdb.TMDb
}

func TMDBInit(tmdbkey string) *TMDb{
	var TMDBCONFIG = tmdb.Config{
		APIKey:   tmdbkey,
		Proxies:  nil,
		UseProxy: false,
	}
	mytmdb := new(TMDb)
	mytmdb.tmdb = tmdb.Init(TMDBCONFIG)
	return mytmdb
}


func (tmdbapi *TMDb)FetchMovieDetails(m *Short) (*Short, error) {
	var options = make(map[string]string)
	options["language"] = "ru"
	options["year"] = m.Year
	log.Println("Start TMDB search:", m.Searchname, m.Year)
	r, err := tmdbapi.tmdb.SearchMovie(m.Searchname, options)
	log.Println("Got result for TMDB search:", m.Searchname, m.Year)
	if err != nil {
		return nil, err
	}
	if len(r.Results) > 0 {
		m.MovieShort = r.Results[0]
	}
	// m.ID = int(rand.Int63())
	// m.OriginalTitle="pizda"
	return m, nil
}


func MoviesPipelineStream(ctx context.Context, movies []*Short, tmdbkey string, limit int64) (chan *Short, chan error){
	m, err := pipeline.Producer(ctx, movies)
	if err != nil {
		mc := make(chan *Short)
		ec := make(chan error)
		return mc, ec
	}
	mytmdb := TMDBInit(tmdbkey)
	movie_chan, errors := pipeline.Step(ctx, m, mytmdb.FetchMovieDetails, limit)
	return movie_chan, errors
}

func ChannelToMoviesToDb(ctx context.Context, cancelFunc context.CancelFunc, values <-chan *Short, errors <-chan error, firestoreClient *firestore.Client) int {
	i:=0
	for {
		select {
		case <-ctx.Done():
			log.Print(ctx.Err().Error())
			return i
		case err := <-errors:
			if err != nil {
				log.Println("error: ", err.Error())
				cancelFunc()
			}
		case m, ok := <-values:
			if ok {
				if len(m.OriginalTitle) > 0 {

					m.updateMoviesAttribs()
					m.writeToDb(ctx, firestoreClient)
					i+=1
				}
			} else {
				log.Println("Done! Collected", i, "movies")
				return i
			}
		}
	}
}



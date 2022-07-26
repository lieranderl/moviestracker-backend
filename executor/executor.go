package executor

import (
	"context"
	"log"

	"cloud.google.com/go/firestore"
	"moviestracker/movies"
	"moviestracker/rutor"
	"moviestracker/torrents"
	"moviestracker/tracker"
	"google.golang.org/api/option"
)

type Config struct {
	urls            []string
	tmdbApiKey      string
	firebaseProject string
	goption         option.ClientOption
}

type TrackersPipeline struct {
	torrents []*torrents.Torrent
	movies   []*movies.Short
	config   Config
}

func Init(urls []string, tmdbapikey string, firebaseProject string, firebaseconfig string) *TrackersPipeline {
	tp := new(TrackersPipeline)
	goption := option.WithCredentialsJSON([]byte(firebaseconfig))
	tp.config = Config{urls: urls, tmdbApiKey: tmdbapikey, firebaseProject: firebaseProject, goption: goption}
	return tp
}

func (p *TrackersPipeline) ConvertTorrentsToMovieShort() *TrackersPipeline {
	ms := make([]*movies.Short, 0)
	hash_list := make([]string, 0)
	i := 0
	for _, movietorr := range p.torrents {
		found := false
		for _, h := range hash_list {
			if h == movietorr.Hash {
				found = true
				for _, m := range ms {
					if m.Hash == movietorr.Hash {
						m.Torrents = append(m.Torrents, movietorr)
					}
				}
				break
			}
		}
		if !found {
			hash_list = append(hash_list, movietorr.Hash)
			searchname := ""
			if movietorr.OriginalName != "" {
				searchname = movietorr.OriginalName
			} else {
				searchname = movietorr.RussianName
			}
			movie := new(movies.Short)
			movie.Hash = movietorr.Hash
			movie.Searchname = searchname
			movie.Year = movietorr.Year
			i += 1
			movie.Torrents = append(movie.Torrents, movietorr)
			ms = append(ms, movie)
		}

	}
	p.movies = ms
	return p

}

func (p *TrackersPipeline) TmdbAndFirestore() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firestoreClient, err := firestore.NewClient(ctx, p.config.firebaseProject, p.config.goption)
	if err != nil {
		log.Fatalln(err)
	}
	movieChan, errorChan := movies.MoviesPipelineStream(ctx, p.movies, p.config.tmdbApiKey, 20)
	movies.ChannelToMoviesToDb(ctx, cancel, movieChan, errorChan, firestoreClient)
}

func (p *TrackersPipeline) RunTrackersSearchPipilene() *TrackersPipeline {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	config := tracker.Config{Urls: p.config.urls, TrackerParser: rutor.ParsePage}
	rutorTracker := tracker.Init(config)
	// kinozalTracker := new(tracker.Tracker)
	// kinozalTracker.Url = rutor.Kinoz_URLS
	// kinozalTracker.TrackerParser = rutor.KParsePage
	torrentsResults, rutorErrors := rutorTracker.TorrentsPipelineStream(ctx)
	// torrentsResults2, kinozalErrors := kinozalTracker.BuildTorrentListStream(ctx)
	// allTorrents := pipeline.Merge(ctx, torrentsResults)
	// allErrors := pipeline.Merge(ctx, rutorErrors)
	p.torrents = torrents.MergeTorrentChannlesToSlice(ctx, cancel, torrentsResults, rutorErrors)
	return p
}

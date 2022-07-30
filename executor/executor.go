package executor

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	// "moviestracker/imdbratingupdater"
	"moviestracker/movies"
	"moviestracker/tapochek"
	"moviestracker/torrents"
	"moviestracker/tracker"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type Config struct {
	urls            []string
	tmdbApiKey      string
	firebaseProject string
	goption         option.ClientOption
}

type TrackersPipeline struct {
	Torrents []*torrents.Torrent
	Movies   []*movies.Short
	config   Config
	Errors	 []error
}

func Init(urls []string, tmdbapikey string, firebaseProject string, firebaseconfig string) *TrackersPipeline {
	tp := new(TrackersPipeline)
	goption := option.WithCredentialsJSON([]byte(firebaseconfig))
	tp.config = Config{urls: urls, tmdbApiKey: tmdbapikey, firebaseProject: firebaseProject, goption: goption}
	return tp
}

func(p *TrackersPipeline) DeleteOldMoviesFromDb() *TrackersPipeline {
	if len(p.Errors) > 0 {
		return p
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firestoreClient, err := firestore.NewClient(ctx, p.config.firebaseProject, p.config.goption)
	if err != nil {
		p.Errors = append(p.Errors, err)
	}
	moviesListRef := firestoreClient.Collection("latesttorrentsmovies").Where("ReleaseDate", "<", time.Now().Add(-time.Hour*24*30*6).Format("2006-01-02"))
	iter := moviesListRef.Documents(ctx)
	batch := firestoreClient.Batch()
	numDeleted := 0
	for {
			doc, err := iter.Next()
			if err == iterator.Done {
					break
			}
			if err != nil {
				p.Errors = append(p.Errors, err)
				return p
			}

			batch.Delete(doc.Ref)
			numDeleted++
	}

	// If there are no documents to delete,
	// the process is over.
	if numDeleted == 0 {
			return p
	}

	_, err = batch.Commit(ctx)
	if err != nil {
		p.Errors = append(p.Errors, err)
	}
	return p
}

func (p *TrackersPipeline) ConvertTorrentsToMovieShort() *TrackersPipeline {
	if len(p.Errors) > 0 {
		return p
	}
	ms := make([]*movies.Short, 0)
	hash_list := make([]string, 0)
	i := 0
	for _, movietorr := range p.Torrents {
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
	p.Movies = ms
	return p

}

func (p *TrackersPipeline) TmdbAndFirestore() *TrackersPipeline{
	if len(p.Errors) > 0 {
		return p
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firestoreClient, err := firestore.NewClient(ctx, p.config.firebaseProject, p.config.goption)
	if err != nil {
		p.Errors = append(p.Errors, err)
		return p
	}
	// imdbratingupdater.Imdbratingupdater(ctx, firestoreClient)
	movieChan, errorChan := movies.MoviesPipelineStream(ctx, p.Movies, p.config.tmdbApiKey, 20)
	movies.ChannelToMoviesToDb(ctx, cancel, movieChan, errorChan, firestoreClient)
	return p
}


func (p *TrackersPipeline) RunTrackersSearchPipilene() *TrackersPipeline {
	if len(p.Errors) > 0 {
		return p
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log.Println(p.config.urls)
	// config := tracker.Config{Urls: p.config.urls, TrackerParser: rutor.ParsePage}
	// rutorTracker := tracker.Init(config)
	// kinozalTracker := new(tracker.Tracker)
	// kinozalTracker.Url = rutor.Kinoz_URLS
	// kinozalTracker.TrackerParser = rutor.KParsePage
	// torrentsResults, rutorErrors := rutorTracker.TorrentsPipelineStream(ctx)
	// torrentsResults2, kinozalErrors := kinozalTracker.BuildTorrentListStream(ctx)
	// allTorrents := pipeline.Merge(ctx, torrentsResults)
	// allErrors := pipeline.Merge(ctx, rutorErrors)

	config := tracker.Config{Urls: p.config.urls, TrackerParser: tapochek.ParsePage}
	tapochekTracker := tracker.Init(config)
	torrentsResults, rutorErrors := tapochekTracker.TorrentsPipelineStream(ctx)
	
	ts, err := torrents.MergeTorrentChannlesToSlice(ctx, cancel, torrentsResults, rutorErrors)
	if err != nil {
		p.Errors = append(p.Errors, err)
	} else {
		p.Torrents = ts
	}
	return p
}

func (p *TrackersPipeline)HandleErrors() error{
	var err error
	if len(p.Errors) > 0 {
		errorStrSlice := make([]string,0)
		for _, err := range p.Errors {
			errorStrSlice = append(errorStrSlice, err.Error())
		}
		err := errors.New(strings.Join(errorStrSlice, ",\n"))
		log.Println(err)
	}
	return err
}
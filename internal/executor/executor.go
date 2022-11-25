package executor

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"ex.com/moviestracker/internal/kinozal"
	"ex.com/moviestracker/internal/movies"
	"ex.com/moviestracker/internal/rutor"
	"ex.com/moviestracker/internal/torrents"
	"ex.com/moviestracker/internal/tracker"
	"ex.com/moviestracker/pkg/pipeline"

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
	Errors   []error
}

func Init(urls []string, tmdbapikey string, firebaseProject string, firebaseconfig string, saveToDb bool) *TrackersPipeline {
	tp := new(TrackersPipeline)
	goption := option.WithCredentialsJSON([]byte(firebaseconfig))
	tp.config = Config{urls: urls, tmdbApiKey: tmdbapikey, firebaseProject: firebaseProject, goption: goption}
	return tp
}

func (p *TrackersPipeline) DeleteOldMoviesFromDb() *TrackersPipeline {
	if len(p.Errors) > 0 {
		return p
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firestoreClient, err := firestore.NewClient(ctx, p.config.firebaseProject, p.config.goption)
	if err != nil {
		p.Errors = append(p.Errors, err)
	}
	moviesListRef := firestoreClient.Collection("latesttorrentsmovies").Where("LastTimeFound", "<", time.Now().Add(-time.Hour*24*30*12).Format("2006-01-02T15:04:05.000Z"))
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
			movie.UpdateMoviesAttribs()
			ms = append(ms, movie)
		}

	}
	p.Movies = ms
	return p

}

func (p *TrackersPipeline) Tmdb() *TrackersPipeline {
	if len(p.Errors) > 0 {
		return p
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	movieChan, errorChan := movies.MoviesPipelineStream(ctx, p.Movies, p.config.tmdbApiKey, 20)
	p.Movies = movies.ChannelToMovies(ctx, cancel, movieChan, errorChan)
	return p
}

func (p *TrackersPipeline) SaveToDb(collection string) *TrackersPipeline {
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
	for _, m := range p.Movies{
		m.WriteMovieToDb(ctx, firestoreClient, collection)
	}
	return p
}

func (p *TrackersPipeline) RunTrackersSearchPipilene(isMovie string) *TrackersPipeline {
	if len(p.Errors) > 0 {
		return p
	}

	var config, configKZ tracker.Config
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if isMovie == "true" {
		config = tracker.Config{Urls: []string{p.config.urls[0]}, TrackerParser: rutor.ParseMoviePage}
		configKZ = tracker.Config{Urls: []string{p.config.urls[1]}, TrackerParser: kinozal.ParseMoviePage}
	} else {
		config = tracker.Config{Urls: []string{p.config.urls[0]}, TrackerParser: rutor.ParseSeriesPage}
		configKZ = tracker.Config{Urls: []string{p.config.urls[1]}, TrackerParser: kinozal.ParseSeriesPage}
	}
	rutorTracker := tracker.Init(config)
	kzTracker := tracker.Init(configKZ)

	torrentsResults1, rutorErrors1 := rutorTracker.TorrentsPipelineStream(ctx)
	torrentsResults2, rutorErrors2 := kzTracker.TorrentsPipelineStream(ctx)

	allTorrents := pipeline.Merge(ctx, torrentsResults1, torrentsResults2)
	allErrors := pipeline.Merge(ctx, rutorErrors1, rutorErrors2)

	log.Println("ALL: ", allTorrents)

	ts, err := torrents.MergeTorrentChannlesToSlice(ctx, cancel, allTorrents, allErrors)
	log.Println("TS length before: ", len(ts))
	ts = torrents.RemoveDuplicatesInPlace(ts)
	log.Println("TS length AFTER: ", len(ts))

	log.Println("ALL TORRS: ", ts)
	if err != nil {
		p.Errors = append(p.Errors, err)
	} else {
		p.Torrents = ts
	}
	return p
}

func (p *TrackersPipeline) RunRutorPipiline() *TrackersPipeline {
	if len(p.Errors) > 0 {
		return p
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := tracker.Config{Urls: p.config.urls, TrackerParser: rutor.ParseMoviePage}
	rutorTracker := tracker.Init(config)
	torrentsResults, rutorErrors := rutorTracker.TorrentsPipelineStream(ctx)

	ts, err := torrents.MergeTorrentChannlesToSlice(ctx, cancel, torrentsResults, rutorErrors)
	if err != nil {
		p.Errors = append(p.Errors, err)
	} else {
		p.Torrents = ts
	}
	return p
}

func (p *TrackersPipeline) HandleErrors() error {
	var err error
	if len(p.Errors) > 0 {
		errorStrSlice := make([]string, 0)
		for _, err := range p.Errors {
			errorStrSlice = append(errorStrSlice, err.Error())
		}
		err := errors.New(strings.Join(errorStrSlice, ",\n"))
		log.Println(err)
	}
	return err
}

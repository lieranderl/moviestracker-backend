package tracker

import (
	"context"
	"log"

	"moviestracker/pipeline"
	"moviestracker/torrents"
)

type Config struct {
	Urls []string
	TrackerParser func(string) ([]*torrents.Torrent, error)
}

type Tracker struct {
	urls []string
	trackerParser func(string) ([]*torrents.Torrent, error)
}

func Init(config Config) *Tracker {
	return &Tracker{urls: config.Urls, trackerParser: config.TrackerParser}
}


func (t Tracker)TorrentsPipelineStream(ctx context.Context) (chan []*torrents.Torrent, chan error){
	urlStream, err := pipeline.Producer(ctx, t.urls)
	if err != nil {
		log.Fatal(err)
	}
	torrents_chan, errors := pipeline.Step(ctx, urlStream, t.trackerParser, 10)
	return torrents_chan, errors
}
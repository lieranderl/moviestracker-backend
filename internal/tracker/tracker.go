package tracker

import (
	"context"

	"moviestracker/pkg/pipeline"
	"moviestracker/internal/torrents"
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
		tc := make(chan []*torrents.Torrent,)
		ec := make(chan error)
		return tc, ec
	}
	torrents_chan, errors := pipeline.Step(ctx, urlStream, t.trackerParser, 2)
	return torrents_chan, errors
}
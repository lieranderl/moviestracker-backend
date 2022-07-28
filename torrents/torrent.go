package torrents

import (
	"context"
	"log"
)

type Torrent struct {
	Name		 string
	DetailsUrl 	 string
	OriginalName string
	RussianName  string
	Year         string
	Size         float32
	Magnet       string
	Date         string
	K4           bool
	FHD          bool
	HDR          bool
	DV           bool
	Seeds        int32
	Leeches      int32
	Hash         string
}



func MergeTorrentChannlesToSlice(ctx context.Context, cancelFunc context.CancelFunc, values <-chan []*Torrent, errors <-chan error) ([]*Torrent, error) {
	torrents:= make([]*Torrent,0)
	for {
		select {
		case <-ctx.Done():
			log.Print(ctx.Err().Error())
			return torrents, ctx.Err()
		case err := <-errors:
			if err != nil {
				log.Println("error: ", err.Error())
				cancelFunc()
				return torrents, err
			}
		case res, ok := <-values:
			if ok {
				torrents = append(torrents, res...)
			} else {
				log.Print("done")
				return torrents, nil
			}
		}
	}
}



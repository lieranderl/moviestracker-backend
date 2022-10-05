package torrents

import (
	"context"
	"log"
	"sort"
	"strings"
)

type Torrent struct {
	Name         string
	DetailsUrl   string
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
	MagnetHash   string
}

func MergeTorrentChannlesToSlice(ctx context.Context, cancelFunc context.CancelFunc, values <-chan []*Torrent, errors <-chan error) ([]*Torrent, error) {
	torrents := make([]*Torrent, 0)
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

func RemoveDuplicatesInPlace(torrents []*Torrent) []*Torrent {
	// if there are 0 or 1 items we return the slice itself.
	if len(torrents) < 2 {
		return torrents
	}

	// make the slice ascending sorted.
	sort.Slice(torrents, func(i, j int) bool {
		return strings.ToLower(torrents[i].MagnetHash) < strings.ToLower(torrents[j].MagnetHash)
	})

	uniqPointer := 0

	for i := 1; i < len(torrents); i++ {
		// compare a current item with the item under the unique pointer.
		// if they are not the same, write the item next to the right of the unique pointer.
		if !strings.EqualFold(strings.ToLower(torrents[uniqPointer].MagnetHash), strings.ToLower(torrents[i].MagnetHash)) {
			uniqPointer++
			torrents[uniqPointer] = torrents[i]
		}
	}

	return torrents[:uniqPointer+1]
}

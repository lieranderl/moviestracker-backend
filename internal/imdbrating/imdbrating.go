package imdbrating

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)


type imdbRating struct {
	Id     string
	Rating string
	Votes  string
}

func storageReader(ctx context.Context) (*storage.Reader, error) {
	storageClient, err := storage.NewClient(ctx, option.WithCredentialsJSON([]byte(os.Getenv("FIREBASECONFIG"))))
	if err != nil {
		return nil, err
	}
	bkt := storageClient.Bucket(os.Getenv("FIREBASE_BUCKET"))
	file := bkt.Object(os.Getenv("FIREBASE_IMDB_FILE"))
	reader, err := file.NewReader(ctx)
	if err != nil {
		return nil, err
	}
	return reader, nil
}


func GetImdbRatingById(imdb_id string) ([]byte, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := make([]byte,0)
	reader, err := storageReader(ctx)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

    fileScanner := bufio.NewScanner(reader)
    fileScanner.Split(bufio.ScanLines)
    for fileScanner.Scan() {
    	if strings.Contains(fileScanner.Text(), imdb_id) {
			split := strings.Split(fileScanner.Text(), "	")
			if (len(split)==3) {
				imdb := new(imdbRating)
				imdb.Id = split[0]
				imdb.Rating = split[1]
				imdb.Votes = split[2]
				b, err = json.Marshal(imdb)
				if err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
			break
		}
    }
	return b, nil
}

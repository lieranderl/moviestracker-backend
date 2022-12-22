package imdbrating

import (
	"compress/gzip"
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

const (
	URLBASE        = "https://datasets.imdbws.com/"
	RATINGSFILEARC = "title.ratings.tsv.gz"
)

func storageWriter(ctx context.Context) (*storage.Writer, error) {
	storageClient, err := storage.NewClient(ctx, option.WithCredentialsJSON([]byte(os.Getenv("FIREBASECONFIG"))))
	if err != nil {
		return nil, err
	}
	bkt := storageClient.Bucket(os.Getenv("FIREBASE_BUCKET"))
	file := bkt.Object(os.Getenv("FIREBASE_IMDB_FILE"))
	writer := file.NewWriter(ctx)
	return writer, nil
}

func getImdbFile() *http.Response {
	url := URLBASE + RATINGSFILEARC
	start := time.Now()
	headResp, err := http.Head(url)
	if err != nil {
		panic(err)
	}

	defer headResp.Body.Close()

	size, err := strconv.Atoi(headResp.Header.Get("Content-Length"))
	if err != nil {
		log.Println(err)
	}
	sizeMb := float64(size) * 0.000001

	//Get data
	resp, err := http.Get(url)
	if err != nil {
		log.Println(err)
	}

	if resp.StatusCode == 200 {
		elapsed := time.Since(start)
		log.Printf("%0.3f MB downloaded completed in %s", sizeMb, elapsed)
		if err != nil {
			log.Println(err)
		}
		return resp
	}

	return nil
}

func DownloadImdb() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp := getImdbFile()
	if resp == nil {
		log.Fatalln("Empty file")
	}
	archive, err := gzip.NewReader(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}
	defer archive.Close()
	defer resp.Body.Close()
	var p []byte
	p, err = io.ReadAll(archive)
	if err != nil {
		log.Fatalln(err)
	}

	w, err := storageWriter(ctx)
	if err != nil {
		log.Fatalln("Failed to init storage writer")
	}
	defer w.Close()

	_, err = w.Write(p)
	if err != nil {
		log.Fatalln("Failed to write file")
	}

	return err

}

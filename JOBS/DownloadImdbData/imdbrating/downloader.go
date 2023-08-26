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
	// Init storage client. Firestore credentials are stored in FIREBASECONFIG env variable/secret
	storageClient, err := storage.NewClient(ctx, option.WithCredentialsJSON([]byte(os.Getenv("FIREBASECONFIG"))))
	if err != nil {
		return nil, err
	}
	// init storage bucket and file
	bkt := storageClient.Bucket(os.Getenv("FIREBASE_BUCKET"))
	file := bkt.Object(os.Getenv("FIREBASE_IMDB_FILE"))
	// init storage writer
	writer := file.NewWriter(ctx)
	return writer, nil
}

func getImdbFile() *http.Response {
	url := URLBASE + RATINGSFILEARC
	start := time.Now()

	// Probe the Content-Length header by sending an HTTP HEAD request
	headResp, err := http.Head(url)
	if err != nil {
		panic(err)
	}
	defer headResp.Body.Close()

	// Get the size from the Content-Length header
	size, err := strconv.Atoi(headResp.Header.Get("Content-Length"))
	if err != nil {
		log.Println("Failed to get Content-Length header", err)
	}
	sizeMb := float64(size) * 0.000001

	//Get data from url
	resp, err := http.Get(url)
	if err != nil {
		log.Fatalln("Unable to download file.", err)
	}

	// Return response if status code is 200
	if resp.StatusCode == 200 {
		elapsed := time.Since(start)
		log.Printf("%0.3f MB downloaded completed in %s", sizeMb, elapsed)
		return resp
	}
	// Return nil if status code is not 200
	return nil
}

func DownloadImdb() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Get the file from imdbws
	resp := getImdbFile()
	if resp == nil {
		log.Fatalln("Empty file")
	}
	// Make zip reader for resp.Body. Unzip on the fly.
	archive, err := gzip.NewReader(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}
	defer archive.Close()
	defer resp.Body.Close()

	//Read zip file. This is the unzipped file we want to store in storage
	var file []byte
	file, err = io.ReadAll(archive)
	if err != nil {
		log.Fatalln(err)
	}

	//Make storage writer
	w, err := storageWriter(ctx)
	if err != nil {
		log.Fatalln("Failed to init storage writer", err)
	}
	defer w.Close()

	//Write the unzipped file to storage
	n, err := w.Write(file)
	if err != nil {
		log.Fatalln("Failed to write file", err)
	}
	log.Printf("Wrote %d bytes to storage", n)
}

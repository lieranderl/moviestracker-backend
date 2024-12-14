package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/lieranderl/moviestracker-package/executor"
)

func main() {

	if len(os.Args) < 2 {
		log.Println("No arguments provided")
		return
	}
	// godotenv.Load()
	dbname := os.Args[1]
	dbtype := os.Args[2]
	urls := ""
	switch dbname {
	case "latesttorrentsmovies":
		urls = os.Getenv("RUTOR_URLS")
	case "hdr10movies":
		urls = os.Getenv("RUTOR_HDR10_URLS")
	case "dvmovies":
		urls = os.Getenv("RUTOR_DV_URLS")
	default:
		log.Println("Wrong argument provided")
		return
	}

	log.Printf("Start %s !", dbname)
	start := time.Now()

	// Initialize the pipeline

	switch dbtype {
	case "mongo":
		pipeline := executor.InitPipeline(context.Background(), strings.Split(urls, ","), os.Getenv("TMDBAPIKEY"), executor.OptionWithMongo(os.Getenv("MONGO_URI")))
		err := pipeline.
			RunTrackersSearchPipeline(true).
			ConvertTorrentsToMovieShort().
			Tmdb().
			SaveToDb().
			// DeleteOldMoviesFromDb().
			HandleErrors()
		if err != nil {
			return
		}
	case "firebase":
		pipeline := executor.InitPipeline(context.Background(), strings.Split(urls, ","), os.Getenv("TMDBAPIKEY"), executor.OptionWithFirebase(os.Getenv("FIREBASE_PROJECT"), os.Getenv("FIREBASECONFIG")))
		err := pipeline.
			RunTrackersSearchPipeline(true).
			ConvertTorrentsToMovieShort().
			Tmdb().
			SaveToDb().
			// DeleteOldMoviesFromDb().
			HandleErrors()
		if err != nil {
			return
		}
	}

	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)

}

package main

import (
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
	log.Println(os.Args[1])
	dbname := os.Args[1]
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

	//	latesttorrentsmovies	strings.Split(os.Getenv("RUTOR_URLS"), ","),
	//	hdr10movies	strings.Split(os.Getenv("RUTOR_HDR10_URLS"), ","),
	//	dvmovies	strings.Split(os.Getenv("RUTOR_DV_URLS"), ","),

	pipeline := executor.Init(
		strings.Split(urls, ","),
		os.Getenv("TMDBAPIKEY"),
		os.Getenv("FIREBASE_PROJECT"),
		os.Getenv("FIREBASECONFIG"),
		true,
	)
	err := pipeline.
		RunRutorPipiline().
		ConvertTorrentsToMovieShort().
		Tmdb().
		SaveToDb(dbname).
		DeleteOldMoviesFromDb().
		HandleErrors()
	if err != nil {
		return
	}

	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)

}

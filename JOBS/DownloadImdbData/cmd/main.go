package main

import (
	"log"
	"time"

	"moviestracker.net/downloadimdbdata/imdbrating"
)

func main() {
	log.Println("Start DownloadImdbData!")
	start := time.Now()
	if err := imdbrating.DownloadImdb(); err != nil {
		log.Fatalf("download imdb data failed: %v", err)
	}
	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)
}

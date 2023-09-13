package main

import (
	"log"
	"time"

	"moviestracker.net/downloadimdbdata/imdbrating"
)

func main() {
	log.Println("Start DownloadImdbData!")
	start := time.Now()
	imdbrating.DownloadImdb()
	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)
}

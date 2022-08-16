package main

import (
	"log"
	"time"
	"moviestracker/internal/imdbrating"

	"github.com/joho/godotenv"
)

func main() {
	// 	/////////Manual run

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	start := time.Now()
	imdbrating.DownloadImdbData()
	elapsed := time.Since(start)
	log.Printf("Took %s", elapsed)

	// 	////////////////////////
	// 	/////////for AWS lambda
	// start := time.Now()
	// lambda.Start(DownloadImdbData)
	// elapsed := time.Since(start)
	// log.Printf("Took %s", elapsed)

}

package main

import (
	"log"
	"ex.com/moviestracker/internal/imdbrating"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	// "github.com/joho/godotenv"
)

func main() {
	// 	/////////Manual run

	// err := godotenv.Load()
	// if err != nil {
	// 	log.Fatal("Error loading .env file")
	// }
	// start := time.Now()
	// imdbrating.DownloadImdbData()
	// elapsed := time.Since(start)
	// log.Printf("Took %s", elapsed)

	// 	////////////////////////
	/////////for AWS lambda
	start := time.Now()
	lambda.Start(imdbrating.DownloadImdbData)
	elapsed := time.Since(start)
	log.Printf("Took %s", elapsed)

}

package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"moviestracker/internal/executor"
	
	// "github.com/joho/godotenv"
	"github.com/aws/aws-lambda-go/lambda"
)



func CollectLatestMoviesHandler() (string, error) {
	fmt.Println("Start Test_func!")
	start := time.Now()
	pipeline := executor.Init(
	strings.Split(os.Getenv("RUTOR_URLS"), ","), 
				  os.Getenv("TMDBAPIKEY"), 
				  os.Getenv("FIREBASE_PROJECT"), 
				  os.Getenv("FIREBASECONFIG"))
	err := pipeline.
			RunRutorPipiline().
			ConvertTorrentsToMovieShort().
			TmdbAndFirestore().
			//  DeleteOldMoviesFromDb().
			 HandleErrors()
	if err != nil {
		return "Failed!", err
	}
	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)
	return "Done!", nil
}



func main() {
	/////////Manual run
	// err := godotenv.Load()
    // if err != nil {
    //     log.Fatal("Error loading .env file")
    // }
	// CollectLatestMoviesHandler()

	////////////////////////	
	/////////for AWS lambda
	lambda.Start(CollectLatestMoviesHandler)
}

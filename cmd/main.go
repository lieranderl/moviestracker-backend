package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"moviestracker/executor"

	"github.com/joho/godotenv"
)



func HandleRequest(urls []string, tmdbApiKey, firebaseProject, firebaseConfig string) (string, error) {
	fmt.Println("Start Test_func!")
	start := time.Now()
	pipeline := executor.Init(urls, tmdbApiKey, firebaseProject, firebaseConfig)
	pipeline.DeleteOldMoviesFromDb().
			 RunTrackersSearchPipilene().
			 ConvertTorrentsToMovieShort().
			 TmdbAndFirestore()
	if len(pipeline.Errors) > 0 {
		errorStrSlice := make([]string,0)
		for _, err := range pipeline.Errors {
			errorStrSlice = append(errorStrSlice, err.Error())
		}
		err := errors.New(strings.Join(errorStrSlice, ",\n"))
		log.Println(err)
		return "Failed!", err
	}
	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)
	return "Done!", nil

}

func main() {
	/////////Manual run

	err := godotenv.Load()
    if err != nil {
        log.Fatal("Error loading .env file")
    }
	HandleRequest(
		strings.Split(os.Getenv("RUTOR_URLS"), ","), 
		os.Getenv("TMDBAPIKEY"), 
		os.Getenv("FIREBASE_PROJECT"), 
		os.Getenv("FIREBASECONFIG"))
	
	////////////////////////	
	/////////for AWS lambda

	// lambda.Start(HandleRequest)


}

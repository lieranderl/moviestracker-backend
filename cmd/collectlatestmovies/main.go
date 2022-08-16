package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"moviestracker/internal/executor"
	
	"github.com/joho/godotenv"
	"github.com/aws/aws-lambda-go/lambda"
	// "github.com/joho/godotenv"
)



func CollectLatestMoviesHandler() (string, error) {
	fmt.Println("Start Test_func!")
	start := time.Now()
	pipeline := executor.Init(
	strings.Split(os.Getenv("T_URLS"), ","), 
				  os.Getenv("TMDBAPIKEY"), 
				  os.Getenv("FIREBASE_PROJECT"), 
				  os.Getenv("FIREBASECONFIG"))
	err := pipeline.
			 RunTapochekPipilene().
			 ConvertTorrentsToMovieShort().
			 TmdbAndFirestore().
			 DeleteOldMoviesFromDb().
			 HandleErrors()
	if err != nil {
		return "Failed!", err
	}
	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)
	return "Done!", nil
}


// type Search struct {
// 	MovieName string
// 	Year string
// }

// func TorrentsForMovieHandler(apiRequest events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
// 	log.Println("Start TorrentsForMovieHandler!")
// 	start := time.Now()

// 	if apiRequest.QueryStringParameters["MovieName"] != "" {
// 		pipeline := executor.Init(
// 			[]string{fmt.Sprintf(os.Getenv("RUTOR_SEARCH_URL"), apiRequest.QueryStringParameters["MovieName"], apiRequest.QueryStringParameters["Year"])}, 
// 			"", 
// 			"", 
// 			"")
// 		err := pipeline.RunTrackersSearchPipilene().HandleErrors()
// 		if err != nil {
// 			return events.APIGatewayProxyResponse{Body: err.Error(), StatusCode: 500}, nil
// 		}
// 		elapsed := time.Since(start)
// 		log.Printf("ALL took %s", elapsed)
// 		b, err :=json.Marshal(pipeline.Torrents)
// 		if err != nil {
// 			return events.APIGatewayProxyResponse{Body: err.Error(), StatusCode: 500}, nil
// 		}
// 		return events.APIGatewayProxyResponse{Body: string(b), StatusCode: 200}, nil
				
// 	}
// 	return events.APIGatewayProxyResponse{Body: "Empty request", StatusCode: 500}, nil
	
// }

// func ImdbRatingForId(apiRequest events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
// 	log.Println("Start ImdbRatingForId!")
// 	start := time.Now()
// 	imdb_id := apiRequest.QueryStringParameters["imdb_id"]
	
// 	err := pipeline.RunTrackersSearchPipilene().HandleErrors()
// 	if err != nil {
// 		return events.APIGatewayProxyResponse{Body: err.Error(), StatusCode: 500}, nil
// 	}
// 	elapsed := time.Since(start)
// 	log.Printf("ALL took %s", elapsed)
// 	b, err :=json.Marshal(pipeline.Torrents)
// 	if err != nil {
// 		return events.APIGatewayProxyResponse{Body: err.Error(), StatusCode: 500}, nil
// 	}
// 	return events.APIGatewayProxyResponse{Body: string(b), StatusCode: 200}, nil
// }


func main() {
	/////////Manual run
	err := godotenv.Load()
    if err != nil {
        log.Fatal("Error loading .env file")
    }
	CollectLatestMoviesHandler()

	////////////////////////	
	/////////for AWS lambda
	lambda.Start(CollectLatestMoviesHandler)
}

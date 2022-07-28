package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"moviestracker/executor"
	// "moviestracker/torrents"

	"github.com/aws/aws-lambda-go/events"
	// "github.com/aws/aws-lambda-go/lambda"
	"github.com/joho/godotenv"
	// "github.com/joho/godotenv"
)



func CollectLatestMoviesHandler() (string, error) {
	fmt.Println("Start Test_func!")
	start := time.Now()
	pipeline := executor.Init(
	strings.Split(os.Getenv("RUTOR_URLS"), ","), 
				  os.Getenv("TMDBAPIKEY"), 
				  os.Getenv("FIREBASE_PROJECT"), 
				  os.Getenv("FIREBASECONFIG"))
	err := pipeline.DeleteOldMoviesFromDb().
			 RunTrackersSearchPipilene().
			 ConvertTorrentsToMovieShort().
			 TmdbAndFirestore().
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

func TorrentsForMovieHandler(apiRequest events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	log.Println("Start Test_func!")
	start := time.Now()


	pipeline := executor.Init(
				[]string{fmt.Sprintf(os.Getenv("RUTOR_SEARCH_URL"), apiRequest.QueryStringParameters["MovieName"], apiRequest.QueryStringParameters["Year"])}, 
				"", 
				"", 
				"")
	err := pipeline.RunTrackersSearchPipilene().HandleErrors()
	if err != nil {
		return events.APIGatewayProxyResponse{Body: err.Error(), StatusCode: 500}, nil
	}
	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)
	b, err :=json.Marshal(pipeline.Torrents)
	if err != nil {
		return events.APIGatewayProxyResponse{Body: err.Error(), StatusCode: 500}, nil
	}
	return events.APIGatewayProxyResponse{Body: string(b), StatusCode: 200}, nil
}


func main() {
	/////////Manual run

	err := godotenv.Load()
    if err != nil {
        log.Fatal("Error loading .env file")
    }
	CollectLatestMoviesHandler()

	////MANUAL TorrentsForMovieHandler
	// search := events.APIGatewayProxyRequest{QueryStringParameters: map[string]string{"MovieName":"Девушка в поезде","Year":"2016"}}
	// res, err := TorrentsForMovieHandler(search)
	// if err != nil {
	// 	fmt.Println("ERROR:")
	// 	fmt.Println(err)
	// }
	// fmt.Printf(res.Body)
	///////
	

	

	////////////////////////	
	/////////for AWS lambda

	// lambda.Start(CollectLatestMoviesHandler)
	//lambda.Start(TorrentsForMovieHandler)


}

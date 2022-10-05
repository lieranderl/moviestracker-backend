package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"moviestracker/internal/executor"

	"github.com/aws/aws-lambda-go/events"
	// "github.com/aws/aws-lambda-go/lambda"
	"github.com/joho/godotenv"
	// "github.com/joho/godotenv"
)


func TorrentsForMovieHandler(apiRequest events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	log.Println("Start TorrentsForMovieHandler!")
	start := time.Now()

	if (apiRequest.QueryStringParameters["MovieName"] != "" || apiRequest.QueryStringParameters["Year"] != "" || apiRequest.QueryStringParameters["isMovie"]!="")  {
		pipeline := executor.Init(
			[]string{
				fmt.Sprintf(os.Getenv("RUTOR_SEARCH_URL"), apiRequest.QueryStringParameters["MovieName"], apiRequest.QueryStringParameters["Year"]),
				fmt.Sprintf(os.Getenv("KZ_SEARCH_URL"), apiRequest.QueryStringParameters["MovieName"], apiRequest.QueryStringParameters["Year"]),
			}, 
			"", 
			"", 
			"")
		err := pipeline.RunTrackersSearchPipilene(apiRequest.QueryStringParameters["isMovie"]).HandleErrors()
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
	return events.APIGatewayProxyResponse{Body: "Empty request", StatusCode: 500}, nil
	
}




func main() {
	/////////Manual run

	err := godotenv.Load()
    if err != nil {
        log.Fatal("Error loading .env file")
    }
	
	search := events.APIGatewayProxyRequest{QueryStringParameters: map[string]string{"MovieName":"Вышка","Year":"2022","isMovie":"true"}}
	res, err := TorrentsForMovieHandler(search)
	if err != nil {
		fmt.Println("ERROR:")
		fmt.Println(err)
	}
	fmt.Printf(res.Body)


	search = events.APIGatewayProxyRequest{QueryStringParameters: map[string]string{"MovieName":"дом дракона","Year":"2022","isMovie":"false"}}
	res, err = TorrentsForMovieHandler(search)
	if err != nil {
		fmt.Println("ERROR:")
		fmt.Println(err)
	}
	fmt.Printf(res.Body)


	///

	////////////////////////	
	/////////for AWS lambda
	// lambda.Start(TorrentsForMovieHandler)


}

package main

import (
	// "log"
	"ex.com/moviestracker/internal/imdbrating"

	// "github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	// "github.com/joho/godotenv"
)

func main() {
	/////////Manual run

	// err := godotenv.Load()
	// if err != nil {
	//     log.Fatal("Error loading .env file")
	// }
	// search := events.APIGatewayProxyRequest{QueryStringParameters: map[string]string{"imdb_id":"tt1877830"}}
	// res, err := imdbrating.GetImdbRating(search)
	// if err != nil {
	// 	log.Println("ERROR:")
	// 	log.Println(err)
	// }
	// log.Printf(res.Body)
	///

	////////////////////////
	/////////for AWS lambda

	lambda.Start(imdbrating.GetImdbRating)

}

package main

import (
	"context"
	"log"
	"os"

	"ex.com/moviestracker"
	"github.com/GoogleCloudPlatform/functions-framework-go/funcframework"
	"github.com/joho/godotenv"
)
func main() {
	ctx := context.Background()

	err := godotenv.Load()
	if err != nil {
	    log.Fatal("Error loading .env file")
	}

	if err := funcframework.RegisterHTTPFunctionContext(ctx, "/gettorrents", googlefunc.TorrentsForMovieHandler); err != nil {
		log.Fatalf("funcframework.RegisterHTTPFunctionContext: %v\n", err)
	}

	if err := funcframework.RegisterHTTPFunctionContext(ctx, "/gethdr10", googlefunc.Gethdr10movies); err != nil {
		log.Fatalf("funcframework.RegisterHTTPFunctionContext: %v\n", err)
	}
	// Use PORT environment variable, or default to 8080.
	port := "8080"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}
	if err := funcframework.Start(port); err != nil {
		log.Fatalf("funcframework.Start: %v\n", err)
	}
}
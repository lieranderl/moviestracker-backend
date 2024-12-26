// SERVICES/GetTorrentsForMovie/cmd/main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lieranderl/moviestracker-package/executor"
)

func main() {
	http.HandleFunc("/", handleRequest) // Extracted handler function
	log.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("ListenAndServe: %v", err)
	}
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	moviename := r.URL.Query().Get("MovieName")
	year := r.URL.Query().Get("Year")
	ismovie := r.URL.Query().Get("isMovie")

	isboolmovie := strings.ToLower(ismovie) == "true"

	log.Println(moviename, year, ismovie)

	if moviename == "" && year == "" && ismovie == "" {
		http.Error(w, "Empty Request", http.StatusBadRequest)
		return
	}

	urls := []string{
		fmt.Sprintf(os.Getenv("RUTOR_SEARCH_URL"), moviename, year),
		fmt.Sprintf(os.Getenv("KZ_SEARCH_URL"), moviename, year),
	}
	pipeline := executor.InitPipeline(context.Background(), urls, os.Getenv("TMDBAPIKEY"))
	if err := pipeline.RunTrackersSearchPipeline(isboolmovie).HandleErrors(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)
	torrents := pipeline.GetTorrents()
	log.Printf("Number of torrents returned: %d", len(torrents))

	if len(torrents) == 0 {
		http.Error(w, "No torrents found", http.StatusNotFound)
		return
	}

	if err := json.NewEncoder(w).Encode(torrents); err != nil { // Using json.NewEncoder for better performance
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

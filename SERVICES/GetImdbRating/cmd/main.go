package main

import (
	"log"
	"net/http"

	"moviestracker.net/getimdbrating/imdbrating"
)

func main() {
	http.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		imdb_id := r.URL.Query().Get("imdb_id")
		if imdb_id == "" {
			http.Error(w, "Empty imdb id request", http.StatusBadRequest)
			return
		}

		b, err := imdbrating.GetImdbRatingById(imdb_id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("Retrieved IMDb rating for ID: %s, Result: %s", imdb_id, string(b))

		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	})

	log.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("ListenAndServe: %v", err)
	}
}

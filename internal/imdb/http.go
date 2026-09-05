package imdb

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

var imdbIDPattern = regexp.MustCompile(`^tt[0-9]+$`)

type LookupFunc func(string) (Rating, bool)

func NewHandler(lookup LookupFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("imdb_id"))
		if id == "" {
			http.Error(w, "imdb_id is required", http.StatusBadRequest)
			return
		}
		if !imdbIDPattern.MatchString(id) {
			http.Error(w, "invalid imdb_id format", http.StatusBadRequest)
			return
		}
		rating, ok := lookup(id)
		if !ok {
			http.Error(w, ErrRatingNotFound.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(rating); err != nil {
			http.Error(w, "failed to encode imdb rating", http.StatusInternalServerError)
		}
	})
}

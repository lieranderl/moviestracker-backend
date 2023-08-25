package googlefunc

import (
	"log"
	"net/http"
	"time"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"moviestracker.net/getimdbrating/imdbrating"
)

func init() {
	functions.HTTP("GetImdbRating", GetImdbRating)
}

func GetImdbRating(w http.ResponseWriter, r *http.Request) {
	log.Println("Start GetImdb!")
	start := time.Now()

	w.Header().Add("Access-Control-Allow-Origin", "*")
	imdb_id := r.URL.Query().Get("imdb_id")

	if imdb_id == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Empty imdb id request"))
		return
	}
	b, err := imdbrating.GetImdbRatingById(imdb_id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	} else {
		w.Write(b)
	}

	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)
}

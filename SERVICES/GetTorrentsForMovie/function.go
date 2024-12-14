package googlefunc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"github.com/lieranderl/moviestracker-package/executor"
)

func init() {
	functions.HTTP("TorrentsForMovieHandler", TorrentsForMovieHandler)
}

func TorrentsForMovieHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Start TorrentsForMovieHandler!")
	start := time.Now()

	w.Header().Add("Access-Control-Allow-Origin", "*")

	moviename := r.URL.Query().Get("MovieName")
	year := r.URL.Query().Get("Year")
	ismovie := r.URL.Query().Get("isMovie")

	isboolmovie := false
	if strings.ToLower(ismovie) == "true" {
		isboolmovie = true
	}

	log.Println(moviename, year, ismovie)

	if moviename != "" || year != "" || ismovie != "" {
		urls := []string{fmt.Sprintf(os.Getenv("RUTOR_SEARCH_URL"), moviename, year),
			fmt.Sprintf(os.Getenv("KZ_SEARCH_URL"), moviename, year)}
		pipeline := executor.InitPipeline(context.Background(), urls, os.Getenv("TMDBAPIKEY"))
		err := pipeline.RunTrackersSearchPipeline(isboolmovie).HandleErrors()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		}
		elapsed := time.Since(start)
		log.Printf("ALL took %s", elapsed)
		b, err := json.Marshal(pipeline.GetTorrents())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		w.Write(b)

	} else {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Empty Request"))
		return
	}
}

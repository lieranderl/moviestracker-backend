package googlefunc

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"ex.com/moviestracker/internal/executor"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
)

func init() {
	functions.HTTP("TorrentsForMovieHandler", TorrentsForMovieHandler)
	functions.HTTP("Gethdr10movies", Gethdr10movies)
}

func TorrentsForMovieHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Start TorrentsForMovieHandler!")
	start := time.Now()

	w.Header().Add("Access-Control-Allow-Origin", "*")

	moviename := r.URL.Query().Get("MovieName")
	year := r.URL.Query().Get("Year")
	ismovie := r.URL.Query().Get("isMovie")

	log.Println(moviename, year, ismovie)

	if moviename != "" || year != "" || ismovie != "" {
		pipeline := executor.Init(
			[]string{
				fmt.Sprintf(os.Getenv("RUTOR_SEARCH_URL"), moviename, year),
				fmt.Sprintf(os.Getenv("KZ_SEARCH_URL"), moviename, year),
			},
			"",
			"",
			"",
			false,
		)
		err := pipeline.RunTrackersSearchPipilene(ismovie).HandleErrors()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		}
		elapsed := time.Since(start)
		log.Printf("ALL took %s", elapsed)
		b, err := json.Marshal(pipeline.Torrents)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		}
		w.Write(b)

	} else {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 - Empty Request"))
	}
}

func Gethdr10movies(w http.ResponseWriter, r *http.Request){
	log.Println("Start Gethdr10movies!")
	start := time.Now()

	w.Header().Add("Access-Control-Allow-Origin", "*")

	pipeline := executor.Init(
		strings.Split(os.Getenv("RUTOR_HDR10_URLS"), ","),
		os.Getenv("TMDBAPIKEY"),
		os.Getenv("FIREBASE_PROJECT"),
		os.Getenv("FIREBASECONFIG"),
		true,
	)
	err := pipeline.
		RunRutorPipiline().
		ConvertTorrentsToMovieShort().
		Tmdb().
		SaveToDb("hdr10movies").
		HandleErrors()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 - Error"))
	}
	
	// b, err := json.Marshal(pipeline.Movies)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	}
	// w.Write(b)
	w.Write([]byte("200 - OK"))

	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)
}

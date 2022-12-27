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
	"ex.com/moviestracker/internal/imdbrating"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
)

func init() {
	functions.HTTP("CollectLatestMovies", CollectLatestMovies)
	functions.HTTP("TorrentsForMovieHandler", TorrentsForMovieHandler)
	functions.HTTP("Gethdr10movies", Gethdr10movies)
	functions.HTTP("GetImdbRating", GetImdbRating)
	functions.HTTP("DownloadImdbData", DownloadImdbData)
	functions.HTTP("Getdvmovies", Getdvmovies)
}


func CollectLatestMovies(w http.ResponseWriter, r *http.Request) {
	log.Println("Start CollectLatestMovies!")
	start := time.Now()

	w.Header().Add("Access-Control-Allow-Origin", "*")

	pipeline := executor.Init(
		strings.Split(os.Getenv("RUTOR_URLS"), ","),
		os.Getenv("TMDBAPIKEY"),
		os.Getenv("FIREBASE_PROJECT"),
		os.Getenv("FIREBASECONFIG"),
		true,
	)
	err := pipeline.
		RunRutorPipiline().
		ConvertTorrentsToMovieShort().
		Tmdb().
		SaveToDb("latesttorrentsmovies").
		DeleteOldMoviesFromDb().
		HandleErrors()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 - Error"))
		return
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write([]byte("200 - OK"))

	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)
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
			return
		}
		w.Write(b)

	} else {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Empty Request"))
		return
	}
}

func Gethdr10movies(w http.ResponseWriter, r *http.Request) {
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
		return
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write([]byte("200 - OK"))

	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)
}

func Getdvmovies(w http.ResponseWriter, r *http.Request) {
	log.Println("Start Get dv movies!")
	start := time.Now()

	w.Header().Add("Access-Control-Allow-Origin", "*")

	pipeline := executor.Init(
		strings.Split(os.Getenv("RUTOR_DV_URLS"), ","),
		os.Getenv("TMDBAPIKEY"),
		os.Getenv("FIREBASE_PROJECT"),
		os.Getenv("FIREBASECONFIG"),
		true,
	)
	err := pipeline.
		RunRutorPipiline().
		ConvertTorrentsToMovieShort().
		Tmdb().
		SaveToDb("dvmovies").
		HandleErrors()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 - Error"))
		return
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write([]byte("200 - OK"))

	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)
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

func DownloadImdbData(w http.ResponseWriter, r *http.Request) {
	log.Println("Start DownloadImdbData!")
	start := time.Now()
	w.Header().Add("Access-Control-Allow-Origin", "*")
	err := imdbrating.DownloadImdb()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write([]byte("200 - OK"))
	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)
}

package imdbrating

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const defaultDataFilePath = "./store/imdb_data.tsv"

var ErrRatingNotFound = errors.New("imdb rating not found")

type IMDbRating struct {
	ID     string `json:"Id"`
	Rating string
	Votes  string
}

func GetImdbRatingById(imdbID string) ([]byte, error) {
	return GetImdbRatingByIdFromFile(defaultDataFilePath, imdbID)
}

func GetImdbRatingByIdFromFile(dataFilePath, imdbID string) ([]byte, error) {
	if strings.TrimSpace(imdbID) == "" {
		return nil, errors.New("imdb id is required")
	}

	file, err := os.Open(dataFilePath)
	if err != nil {
		return nil, fmt.Errorf("open imdb data file: %w", err)
	}
	defer file.Close()

	fileScanner := bufio.NewScanner(file)
	fileScanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	fileScanner.Split(bufio.ScanLines)

	for fileScanner.Scan() {
		split := strings.Split(fileScanner.Text(), "\t")
		if len(split) != 3 || split[0] != imdbID {
			continue
		}

		result := IMDbRating{
			ID:     split[0],
			Rating: split[1],
			Votes:  split[2],
		}
		payload, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("marshal imdb rating response: %w", err)
		}
		return payload, nil
	}

	if err := fileScanner.Err(); err != nil {
		return nil, fmt.Errorf("scan imdb data file: %w", err)
	}

	return nil, ErrRatingNotFound
}

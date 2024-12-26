package imdbrating

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"strings"
)

type imdbRating struct {
	Id     string
	Rating string
	Votes  string
}

func GetImdbRatingById(imdb_id string) ([]byte, error) {
	file, err := os.Open("./store/imdb_data.tsv")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	dat := make([]byte, 0)
	fileScanner := bufio.NewScanner(reader)
	fileScanner.Split(bufio.ScanLines)
	for fileScanner.Scan() {
		if bytes.Contains(fileScanner.Bytes(), []byte(imdb_id)) {
			split := strings.Split(fileScanner.Text(), "	")
			if len(split) == 3 {
				imdb := new(imdbRating)
				imdb.Id = split[0]
				imdb.Rating = split[1]
				imdb.Votes = split[2]
				dat, err = json.Marshal(imdb)
				if err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
			break
		}
	}
	return dat, nil
}

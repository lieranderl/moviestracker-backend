package kinozal

import (
	"log"
	"testing"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
)

func TestParser(t *testing.T) {
	err := godotenv.Load("../../.env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	name := "Вышка"
	year := "2022"
	test_link := "http://kinozal.tv/browse.php?s=" + name + "%281080p%7C2160p%29&g=3&c=0&v=0&d=" + year + "&w=0&t=0&f=0"
	tors, err := ParseMoviePage(test_link)

	if err == nil {
		for _, tor := range tors {
			if tor.DetailsUrl == "1933025" {
				assert.Equal(t, "magnet:?xt=urn:btih:A8BABAB89BE27F66141477DF58096ECC9721CF59", tor.Magnet)
			}
		}
	} else {
		t.Error()
	}

}

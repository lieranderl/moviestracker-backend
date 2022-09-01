package tapochek

import (
	"crypto/md5"
	"errors"
	"fmt"
	"log"
	"moviestracker/internal/torrents"

	"os"
	"regexp"
	"strings"

	"github.com/gocolly/colly"
)

func ParsePage(url string) ([]*torrents.Torrent, error) {
	var err error
	titles := make([]*torrents.Torrent,0)
	c := colly.NewCollector()	
	c.OnRequest( func(r *colly.Request){r.ResponseCharacterEncoding = "windows-1251"})
	if verifyLogin(c, os.Getenv("T_LOGIN"), os.Getenv("T_PASS")) {
		c.OnHTML("a.torTopic", func(e *colly.HTMLElement) {
			text := e.Text

			log.Println(text)

			m := new(torrents.Torrent)
			r := regexp.MustCompile(`(.*)(\/|\().*\[(\d{4})`)
			g := r.FindStringSubmatch(text)
			name:=strings.TrimSpace(g[1])
			m.Year = g[3]
			if strings.Contains(name, "/") {
				nl := strings.Split(name, "/")
				m.RussianName = strings.TrimSpace(nl[0])
				m.OriginalName = strings.TrimSpace(nl[1])
			} else {
				m.RussianName = name
			}
			if m.OriginalName == "" {
				m.Hash = fmt.Sprintf("%x", md5.Sum([]byte(m.RussianName+m.OriginalName+m.Year)))
			} else {
				m.Hash = fmt.Sprintf("%x", md5.Sum([]byte(m.RussianName+m.Year)))
			}
			titles = append(titles, m)
		})
		err = c.Visit(url)
		return titles, err
	} else {
		return nil, errors.New("tapochek login failed")
	} 

}

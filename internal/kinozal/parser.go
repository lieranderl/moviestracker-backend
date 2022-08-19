package kinozal

import (
	// "crypto/md5"
	// "errors"
	// "fmt"
	"crypto/md5"
	"fmt"

	"moviestracker/internal/torrents"
	"strings"

	// "os"
	// "regexp"
	// "strings"

	"github.com/gocolly/colly"
)

func ParsePage(url string) ([]*torrents.Torrent, error) {
	var err error
	titles := make([]*torrents.Torrent,0)
	c := colly.NewCollector()	
	// c.OnRequest( func(r *colly.Request){r.ResponseCharacterEncoding = "windows-1251"})
	c.OnHTML("div.bx1.stable", func(e *colly.HTMLElement) {
		titles_list := e.ChildAttrs("a","title")
		

		for _,i :=range titles_list {
			rus_name := ""
			year := ""
			
			t_l := strings.Split(i," / ")
			rus_name = strings.Split(i," / ")[0]

			if strings.Contains(rus_name, "(") {
				if strings.Contains(rus_name, "сезон:") {
					continue
				} else {
					rus_name,_,_ = strings.Cut(rus_name, " (")
				}
			}

			rus_name = strings.Trim(rus_name, " ")

			for _,y := range t_l {
				if (len(y) == 4 && strings.Contains(y, "20")) {
					year = y
					break
				}
			}


			m := new(torrents.Torrent)
			m.RussianName = rus_name
			m.OriginalName = rus_name
			m.Year = year
			m.Hash = fmt.Sprintf("%x", md5.Sum([]byte(m.RussianName+m.Year)))
			titles = append(titles, m)
		}

		// m := new(torrents.Torrent)
		// r := regexp.MustCompile(`(.*)(\/|\().*\[(\d{4})`)
		// g := r.FindStringSubmatch(text)
		// name:=strings.TrimSpace(g[1])
		// m.Year = g[3]
		// if strings.Contains(name, "/") {
		// 	nl := strings.Split(name, "/")
		// 	m.RussianName = strings.TrimSpace(nl[0])
		// 	m.OriginalName = strings.TrimSpace(nl[1])
		// } else {
		// 	m.RussianName = name
		// }
		// if m.OriginalName == "" {
		// 	m.Hash = fmt.Sprintf("%x", md5.Sum([]byte(m.RussianName+m.OriginalName+m.Year)))
		// } else {
		// 	m.Hash = fmt.Sprintf("%x", md5.Sum([]byte(m.RussianName+m.Year)))
		// }
		// titles = append(titles, m)
	})
	err = c.Visit(url)
	return titles, err


}

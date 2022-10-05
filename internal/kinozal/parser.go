package kinozal

import (
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"regexp"
	"time"

	"strconv"
	"strings"

	"moviestracker/internal/torrents"

	"github.com/gocolly/colly"
	"github.com/goodsign/monday"
	"github.com/joho/godotenv"
)

func ParseMoviePage(url string) ([]*torrents.Torrent, error) {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	cred := new(Cred)
	cred.Login = os.Getenv("KZ_LOGIN")
	cred.Password = os.Getenv("KZ_PASSWORD")
	var jar *cookiejar.Jar
	tbTransport := &http.Transport{}
	jar, _ = cookiejar.New(&cookiejar.Options{})
	httpcl := &http.Client{Jar: jar, Transport: tbTransport}
	l, httpcl := Login(httpcl, cred)
	titles := make([]*torrents.Torrent, 0)
	c := colly.NewCollector()
	c.OnHTML("tr.bg", func(e *colly.HTMLElement) {
		m := new(kzTorrent)
		m.DetailsUrl = strings.TrimSpace(strings.Split(e.ChildAttr("a", "href"), "id=")[1])
		tds := e.DOM.Children()
		m.Name = tds.Eq(1).Text()
		if !strings.Contains(m.Name, "сезон") {
			m.parseAttributes(m.Name)
			// m.Size = tds.Eq(3).Text()
			if s, err := strconv.ParseFloat(strings.Split(tds.Eq(3).Text(), " ГБ")[0], 32); err == nil {
				m.Size = float32(s)
			}
			// m.Seeds = tds.Eq(4).Text()
			if s, err := strconv.ParseInt(strings.TrimSpace(tds.Eq(4).Text()), 10, 32); err == nil {
				m.Seeds = int32(s)
			}
			if s, err := strconv.ParseInt(strings.TrimSpace(tds.Eq(5).Text()), 10, 32); err == nil {
				m.Leeches = int32(s)
			}

			layout := "2006-01-02T00:00:00.000Z"
			layout_parsed := "02.01.2006"
			timeNow := time.Now()
			yesterday := time.Now().Add(-24 * time.Hour)
			if len(strings.Fields(tds.Eq(6).Text())) > 0 {
				d := strings.Fields(tds.Eq(6).Text())[0]
				if strings.Contains(d, "сегодня") {
					m.Date = monday.Format(timeNow, layout, monday.LocaleRuRU)
				}
				if strings.Contains(d, "вчера") {
					m.Date = monday.Format(yesterday, layout, monday.LocaleRuRU)
				}
				if len(m.Date) == 0 {
					parsedMtime, _ := monday.Parse(layout_parsed, d, monday.LocaleRuRU)
					m.Date = monday.Format(parsedMtime, layout, monday.LocaleRuRU)
				}
			}
			titles = append(titles, &m.Torrent)
		}
	})
	err = c.Visit(url)

	if l {
		titles = fetchMagnetLinks(titles, httpcl)
	}

	return titles, err
}

func ParseSeriesPage(url string) ([]*torrents.Torrent, error) {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	cred := new(Cred)
	cred.Login = os.Getenv("KZ_LOGIN")
	cred.Password = os.Getenv("KZ_PASSWORD")
	var jar *cookiejar.Jar
	tbTransport := &http.Transport{}
	jar, _ = cookiejar.New(&cookiejar.Options{})
	httpcl := &http.Client{Jar: jar, Transport: tbTransport}
	l, httpcl := Login(httpcl, cred)
	titles := make([]*torrents.Torrent, 0)
	c := colly.NewCollector()
	c.OnHTML("tr.bg", func(e *colly.HTMLElement) {
		m := new(kzTorrent)
		m.DetailsUrl = strings.TrimSpace(strings.Split(e.ChildAttr("a", "href"), "id=")[1])
		tds := e.DOM.Children()
		m.Name = tds.Eq(1).Text()

		if strings.Contains(m.Name, "сезон") {
			m.parseAttributes(m.Name)
			// m.Size = tds.Eq(3).Text()
			if s, err := strconv.ParseFloat(strings.Split(tds.Eq(3).Text(), " ГБ")[0], 32); err == nil {
				m.Size = float32(s)
			}
			// m.Seeds = tds.Eq(4).Text()
			if s, err := strconv.ParseInt(strings.TrimSpace(tds.Eq(4).Text()), 10, 32); err == nil {
				m.Seeds = int32(s)
			}
			if s, err := strconv.ParseInt(strings.TrimSpace(tds.Eq(5).Text()), 10, 32); err == nil {
				m.Leeches = int32(s)
			}

			layout := "2006-01-02T00:00:00.000Z"
			layout_parsed := "02.01.2006"
			timeNow := time.Now()
			yesterday := time.Now().Add(-24 * time.Hour)
			if len(strings.Fields(tds.Eq(6).Text())) > 0 {
				d := strings.Fields(tds.Eq(6).Text())[0]
				if strings.Contains(d, "сегодня") {
					m.Date = monday.Format(timeNow, layout, monday.LocaleRuRU)
				}
				if strings.Contains(d, "вчера") {
					m.Date = monday.Format(yesterday, layout, monday.LocaleRuRU)
				}
				if len(m.Date) == 0 {
					parsedMtime, _ := monday.Parse(layout_parsed, d, monday.LocaleRuRU)
					m.Date = monday.Format(parsedMtime, layout, monday.LocaleRuRU)
				}
			}
			titles = append(titles, &m.Torrent)
		}

	})
	err = c.Visit(url)

	if l {
		titles = fetchMagnetLinks(titles, httpcl)
	}

	return titles, err
}

func fetchMagnetLinks(titles []*torrents.Torrent, httpcl *http.Client) []*torrents.Torrent {
	pat := regexp.MustCompile(`btih:([aA-fF,0-9]{40})`)
	magnetChannel := make(chan map[string]string)
	for _, m := range titles {
		go GetMagnet(httpcl, m.DetailsUrl, magnetChannel)
	}
	i := 0
	for mc := range magnetChannel {
		i += 1
		for _, m := range titles {
			if id, ok := mc[m.DetailsUrl]; ok {
				m.Magnet = id
				m.MagnetHash = pat.FindAllStringSubmatch(id, 1)[0][1]
				break
			}
		}
		if i == len(titles) {
			close(magnetChannel)
		}
	}
	return titles
}

type kzTorrent struct {
	torrents.Torrent
}

func (t *kzTorrent) parseAttributes(after string) {
	if strings.Contains(strings.ToLower(after), "2160p") {
		t.K4 = true
	}
	if strings.Contains(strings.ToLower(after), "dolby vision") {
		t.DV = true
	}

	if strings.Contains(after, "1080p") {
		t.FHD = true
	}

	if strings.Contains(strings.ToLower(after), "hdr") {
		t.HDR = true
	}
}

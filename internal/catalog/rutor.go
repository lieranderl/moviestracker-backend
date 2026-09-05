package catalog

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var btihPattern = regexp.MustCompile(`(?i)btih:([a-f0-9]{40})`)

type RutorClient struct {
	client *http.Client
}

func NewRutorClient(client *http.Client) *RutorClient {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &RutorClient{client: client}
}

func (c *RutorClient) Fetch(ctx context.Context, rawURL string, isSeries bool) ([]Torrent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build rutor request: %w", err)
	}
	req.Header.Set("User-Agent", "MoviesTracker/2.0 (+https://moviestracker.net)")
	resp, err := c.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("fetch rutor: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch rutor: unexpected status %s", resp.Status)
	}
	return ParseRutorHTML(resp.Body, isSeries)
}

func ParseRutorHTML(reader io.Reader, isSeries bool) ([]Torrent, error) {
	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return nil, fmt.Errorf("parse rutor html: %w", err)
	}

	result := make([]Torrent, 0)
	doc.Find("tr.gai, tr.tum").Each(func(_ int, row *goquery.Selection) {
		cells := row.Find("td")
		if cells.Length() < 3 {
			return
		}
		links := cells.Eq(1).Find("a")
		magnet, exists := links.FilterFunction(func(_ int, link *goquery.Selection) bool {
			href, _ := link.Attr("href")
			return strings.HasPrefix(strings.ToLower(href), "magnet:")
		}).First().Attr("href")
		if !exists {
			return
		}
		hashMatch := btihPattern.FindStringSubmatch(magnet)
		if len(hashMatch) != 2 {
			return
		}
		titleLink := links.FilterFunction(func(_ int, link *goquery.Selection) bool {
			href, _ := link.Attr("href")
			return href != "" && !strings.HasPrefix(strings.ToLower(href), "magnet:")
		}).Last()
		title := strings.TrimSpace(titleLink.Text())
		if title == "" || isSeries != strings.Contains(title, "[") {
			return
		}

		torrent := parseRutorTitle(title)
		torrent.Magnet = magnet
		torrent.MagnetHash = hashMatch[1]
		torrent.Date = parseRutorDate(strings.TrimSpace(cells.Eq(0).Text()))
		if href, ok := titleLink.Attr("href"); ok {
			if parsed, parseErr := url.Parse(href); parseErr == nil {
				torrent.DetailsURL = (&url.URL{Scheme: "https", Host: "rutor.is"}).ResolveReference(parsed).String()
			}
		}
		parseSizePeers(strings.TrimSpace(cells.Eq(2).Text()), &torrent)
		torrent.Hash = movieHash(torrent)
		result = append(result, torrent)
	})
	return result, nil
}

func parseRutorTitle(title string) Torrent {
	torrent := Torrent{Name: strings.TrimSpace(title)}
	namePart, attributes, found := strings.Cut(title, " (")
	if found {
		attributes = "(" + attributes
	} else {
		namePart = title
	}
	names := strings.Split(namePart, " / ")
	torrent.RussianName = strings.TrimSpace(names[0])
	if len(names) > 1 {
		torrent.OriginalName = strings.TrimSpace(names[len(names)-1])
	} else {
		torrent.OriginalName = torrent.RussianName
	}
	if match := regexp.MustCompile(`\((\d{4})\)`).FindStringSubmatch(attributes); len(match) == 2 {
		torrent.Year = match[1]
	}
	lower := strings.ToLower(" " + attributes + " ")
	torrent.K4 = strings.Contains(lower, "2160p")
	torrent.FHD = strings.Contains(lower, "1080p")
	torrent.DV = strings.Contains(lower, "dolby vision") || strings.Contains(lower, " dolby ")
	torrent.HDR10Plus = strings.Contains(lower, "hdr10+")
	torrent.HDR10 = torrent.HDR10Plus || strings.Contains(lower, "hdr10")
	torrent.HDR = torrent.HDR10 || strings.Contains(lower, " hdr ")
	return torrent
}

func parseRutorDate(raw string) time.Time {
	fields := strings.Fields(raw)
	if len(fields) < 3 {
		return time.Now().UTC()
	}
	months := map[string]time.Month{"янв": time.January, "фев": time.February, "мар": time.March, "апр": time.April, "май": time.May, "июн": time.June, "июл": time.July, "авг": time.August, "сен": time.September, "окт": time.October, "ноя": time.November, "дек": time.December}
	day, dayErr := strconv.Atoi(fields[0])
	year, yearErr := strconv.Atoi(fields[2])
	month, ok := months[strings.ToLower(fields[1])]
	if dayErr != nil || yearErr != nil || !ok {
		return time.Now().UTC()
	}
	if year < 100 {
		year += 2000
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func parseSizePeers(raw string, torrent *Torrent) {
	fields := strings.Fields(raw)
	if len(fields) < 4 {
		return
	}
	if value, err := strconv.ParseFloat(fields[0], 32); err == nil {
		torrent.Size = float32(value)
	}
	if value, err := strconv.ParseInt(fields[len(fields)-2], 10, 32); err == nil {
		torrent.Seeds = int32(value)
	}
	if value, err := strconv.ParseInt(fields[len(fields)-1], 10, 32); err == nil {
		torrent.Leeches = int32(value)
	}
}

func movieHash(torrent Torrent) string {
	key := strings.ToLower(strings.Join([]string{strings.TrimSpace(torrent.RussianName), strings.TrimSpace(torrent.OriginalName), strings.TrimSpace(torrent.Year)}, "|"))
	return fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
}

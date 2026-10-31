package catalog

import (
	"bytes"
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

const maxRutorResponseSize = 4 << 20

var (
	btihPattern = regexp.MustCompile(`(?i)btih:([a-f0-9]{40})`)
	yearPattern = regexp.MustCompile(`\((\d{4})\)`)
	// dvTokenPattern finds the Dolby Vision abbreviations in a lowercased title.
	// Spelled-out "dolby vision" is checked first with a plain substring search.
	dvTokenPattern = regexp.MustCompile(`\b(?:dv|dovi)\b`)
)

// rutorRetryDelay is the pause before the single retry of a failed page fetch.
const rutorRetryDelay = 2 * time.Second

type RutorClient struct {
	client     *http.Client
	retryDelay time.Duration
}

func NewRutorClient(client *http.Client) *RutorClient {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &RutorClient{client: client, retryDelay: rutorRetryDelay}
}

func (c *RutorClient) Fetch(ctx context.Context, rawURL string, isSeries bool) ([]Torrent, error) {
	batch, err := c.FetchBatch(ctx, rawURL, isSeries)
	return batch.Torrents, err
}

// FetchBatch retries a page once when the request fails or Rutor answers with a
// retryable status, because a single transient error otherwise costs the page
// until the next scheduled run. A document that parses but is not a result page
// is not retried: that is a challenge or a changed layout, not a blip.
func (c *RutorClient) FetchBatch(ctx context.Context, rawURL string, isSeries bool) (TrackerBatch, error) {
	const maxAttempts = 2
	var lastErr error
	for attempt := range maxAttempts {
		if attempt > 0 {
			if err := waitForRetry(ctx, c.retryDelay); err != nil {
				return TrackerBatch{}, err
			}
		}
		batch, retryable, err := c.fetchOnce(ctx, rawURL, isSeries)
		if err == nil || !retryable {
			return batch, err
		}
		lastErr = err
	}
	return TrackerBatch{}, lastErr
}

func (c *RutorClient) fetchOnce(ctx context.Context, rawURL string, isSeries bool) (TrackerBatch, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return TrackerBatch{}, false, fmt.Errorf("build rutor request: %w", err)
	}
	req.Header.Set("User-Agent", "MoviesTracker/2.0 (+https://moviestracker.net)")
	resp, err := c.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return TrackerBatch{}, false, ctx.Err()
		}
		return TrackerBatch{}, true, fmt.Errorf("fetch rutor: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError
		return TrackerBatch{}, retryable, fmt.Errorf("fetch rutor: unexpected status %s", resp.Status)
	}
	batch, err := ParseRutorHTMLWithDiagnostics(resp.Body, isSeries)
	return batch, false, err
}

func ParseRutorHTML(reader io.Reader, isSeries bool) ([]Torrent, error) {
	batch, err := ParseRutorHTMLWithDiagnostics(reader, isSeries)
	return batch.Torrents, err
}

func ParseRutorHTMLWithDiagnostics(reader io.Reader, isSeries bool) (TrackerBatch, error) {
	body, err := readBoundedResponse(reader, maxRutorResponseSize)
	if err != nil {
		return TrackerBatch{}, fmt.Errorf("read rutor response: %w", err)
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return TrackerBatch{}, fmt.Errorf("parse rutor html: %w", err)
	}
	rows := doc.Find("tr.gai, tr.tum")
	if rows.Length() == 0 {
		// Captured fixtures establish torrent rows as the page contract. Without
		// an observed empty-page marker, reject rowless responses rather than
		// treating a challenge or changed document as a successful refresh.
		return TrackerBatch{}, fmt.Errorf("unexpected rutor document: torrent rows are missing")
	}

	batch := TrackerBatch{Torrents: make([]Torrent, 0)}
	rows.Each(func(_ int, row *goquery.Selection) {
		cells := row.Find("td")
		if cells.Length() < 3 {
			batch.Fetched++
			batch.RejectedParsing++
			return
		}
		links := cells.Eq(1).Find("a")
		magnet, exists := links.FilterFunction(func(_ int, link *goquery.Selection) bool {
			href, _ := link.Attr("href")
			return strings.HasPrefix(strings.ToLower(href), "magnet:")
		}).First().Attr("href")
		if !exists {
			batch.Fetched++
			batch.RejectedParsing++
			return
		}
		hashMatch := btihPattern.FindStringSubmatch(magnet)
		if len(hashMatch) != 2 {
			batch.Fetched++
			batch.RejectedParsing++
			return
		}
		titleLink := links.FilterFunction(func(_ int, link *goquery.Selection) bool {
			href, _ := link.Attr("href")
			return href != "" && !strings.HasPrefix(strings.ToLower(href), "magnet:")
		}).Last()
		title := strings.TrimSpace(titleLink.Text())
		if title == "" {
			batch.Fetched++
			batch.RejectedParsing++
			return
		}
		if isSeries != strings.Contains(title, "[") {
			batch.SkippedSeries++
			return
		}
		batch.Fetched++

		torrent := parseRutorTitle(title)
		torrent.Magnet = magnet
		torrent.MagnetHash = hashMatch[1]
		date, dateErr := parseRutorDate(strings.TrimSpace(cells.Eq(0).Text()))
		if dateErr != nil {
			batch.RejectedParsing++
			return
		}
		torrent.Date = date
		if href, ok := titleLink.Attr("href"); ok {
			if parsed, parseErr := url.Parse(href); parseErr == nil {
				torrent.DetailsURL = (&url.URL{Scheme: "https", Host: "rutor.is"}).ResolveReference(parsed).String()
			}
		}
		parseSizePeers(strings.TrimSpace(cells.Eq(2).Text()), &torrent)
		torrent.Hash = movieHash(torrent)
		batch.Torrents = append(batch.Torrents, torrent)
	})
	return batch, nil
}

// Limit the decoded HTTP body, including data after a complete HTML/JSON value.
// The HTTP transport may already have decompressed the upstream response.
func readBoundedResponse(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return body, nil
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
	if match := yearPattern.FindStringSubmatch(attributes); len(match) == 2 {
		torrent.Year = match[1]
	}
	// Flags are read from the whole title: some releases, such as box sets
	// written "Title / 1999-2021 / ... Dolby Vision", have no parenthesised
	// attribute section at all.
	lower := strings.ToLower(" " + title + " ")
	torrent.K4 = strings.Contains(lower, "2160p")
	torrent.FHD = strings.Contains(lower, "1080p")
	// A bare "dolby" is not enough: it also names audio, as in "Dolby Atmos".
	torrent.DV = strings.Contains(lower, "dolby vision") || dvTokenPattern.MatchString(lower)
	torrent.HDR10Plus = strings.Contains(lower, "hdr10+")
	torrent.HDR10 = torrent.HDR10Plus || strings.Contains(lower, "hdr10")
	torrent.HDR = torrent.HDR10 || strings.Contains(lower, " hdr ")
	return torrent
}

func parseRutorDate(raw string) (time.Time, error) {
	fields := strings.Fields(raw)
	if len(fields) < 3 {
		return time.Time{}, fmt.Errorf("invalid tracker date %q", raw)
	}
	months := map[string]time.Month{"янв": time.January, "фев": time.February, "мар": time.March, "апр": time.April, "май": time.May, "июн": time.June, "июл": time.July, "авг": time.August, "сен": time.September, "окт": time.October, "ноя": time.November, "дек": time.December}
	day, dayErr := strconv.Atoi(fields[0])
	year, yearErr := strconv.Atoi(fields[2])
	month, ok := months[strings.ToLower(fields[1])]
	if dayErr != nil || yearErr != nil || !ok {
		return time.Time{}, fmt.Errorf("invalid tracker date %q", raw)
	}
	if year < 100 {
		year += 2000
	}
	parsed := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if parsed.Day() != day || parsed.Month() != month || parsed.Year() != year {
		return time.Time{}, fmt.Errorf("invalid tracker date %q", raw)
	}
	return parsed, nil
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

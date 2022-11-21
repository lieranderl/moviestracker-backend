package rutor

import (
	"strconv"
	"strings"

	"ex.com/moviestracker/internal/torrents"
)

type rutorTorrent struct {
	torrents.Torrent
}

func (t *rutorTorrent) rutorTitleToMovie(text string) {
	var after string
	listst := strings.Split(text, "\n")
	ll := strings.Split(listst[1], "  ")

	t.Name = ll[0]
	if isRussianOnly(t.Name) {
		t.RussianName, after, _ = strings.Cut(t.Name, " (")
		t.Year = strings.Split(after, ")")[0]
		t.parseAttributes(after)
	} else {
		t.parseNameAttributes()
	}
	t.Date = parseDate(listst[0])

	t.parseSizePeers(listst[2])
}

func (t *rutorTorrent) parseNameAttributes() {
	var after string
	if isRussianOnly(t.Name) {
		t.RussianName, after, _ = strings.Cut(t.Name, " (")
	} else {
		list := strings.Split(t.Name, " / ")
		l := len(list)
		t.RussianName = list[0]
		t.OriginalName, after, _ = strings.Cut(list[l-1], " (")
	}
	t.Year = strings.Split(after, ")")[0]
	t.parseAttributes(after)
}

func (t *rutorTorrent) parseAttributes(after string) {
	if strings.Contains(strings.ToLower(after), " 2160p ") {
		t.K4 = true
	}
	if strings.Contains(strings.ToLower(after), " dolby ") {
		t.DV = true
	}

	if strings.Contains(after, " 1080p ") {
		t.FHD = true
	}

	if strings.Contains(strings.ToLower(after), " hdr ") || strings.Contains(strings.ToLower(after), " hdr10 ") || strings.Contains(strings.ToLower(after), " hdr10+ ") {
		t.HDR = true
	}

	if strings.Contains(strings.ToLower(after), " hdr10 ") || strings.Contains(strings.ToLower(after), " hdr10+ ")  {
		t.HDR10 = true
	}

	if strings.Contains(strings.ToLower(after), " hdr10+ ") {
		t.HDR10plus = true
	}

	
}

func (t *rutorTorrent) parseSizePeers(text string) {
	list := strings.Fields(text)
	if s, err := strconv.ParseFloat(list[0], 32); err == nil {
		t.Size = float32(s)
	}
	if s, err := strconv.ParseInt(strings.TrimSpace(list[2]), 10, 32); err == nil {
		t.Seeds = int32(s)
	}
	if s, err := strconv.ParseInt(strings.TrimSpace(list[3]), 10, 32); err == nil {
		t.Leeches = int32(s)
	}
}

package catalog

import (
	"strconv"
	"strings"
	"unicode"
)

// Candidate is one TMDB search result reduced to the fields that decide whether
// it describes the same film as the scraped release.
type Candidate struct {
	OriginalTitle string
	ReleaseYear   string
	Title         string
	VoteCount     int64
	// Rank is the position TMDB returned the candidate in. TMDB's own relevance
	// ordering is a real signal, especially for films whose canonical title is
	// in neither the tracker's language nor ours.
	Rank int
}

// confidence describes how strongly a candidate is believed to be the same film.
type confidence int

const (
	confidenceNone confidence = iota
	// confidenceWeak means the title comparison was uninformative but TMDB
	// ranked this candidate first for a year-constrained query and the release
	// year agrees exactly. This is what resolves films published under a title
	// in a third language, such as the Portuguese "Magalhães" for "Magellan",
	// where TMDB matched on an alternative title we never see.
	confidenceWeak
	// confidenceFuzzy means the titles agree once punctuation and case are
	// normalised away, allowing for abbreviation such as "Mulholland Dr."
	// against "Mulholland Drive".
	confidenceFuzzy
	// confidenceStrong means the normalised titles are equal.
	confidenceStrong
)

// yearAgreement records how the scraped year relates to the TMDB release year.
// Release years drift by one routinely, because trackers tend to use the
// festival or production year while TMDB records the wide release.
type yearAgreement int

const (
	yearConflict yearAgreement = iota
	yearUnknown
	yearAdjacent
	yearExact
)

// minPrefixTitleLength guards abbreviation matching. Without a floor, "the cut"
// would be treated as a near match for "the cutting edge"; thirteen characters
// of "mulholland dr" against "mulholland drive" is a real signal, seven
// characters of a common phrase is not.
const minPrefixTitleLength = 8

// minTokenSimilarity is the Dice coefficient over title word sets above which
// two titles are treated as near matches.
const minTokenSimilarity = 0.8

type assessment struct {
	confidence confidence
	rank       int
	votes      int64
	year       yearAgreement
}

// betterThan orders assessments so the strongest evidence wins: confidence
// first, then how well the year agrees, then TMDB's ranking, then vote count.
//
// The year comparison is what makes a duplicate-title search resolve correctly.
// Searching "The Cut" for 2024 returns a 2025 release first and the 2024
// release second; both match the title exactly, so the year is the tiebreak.
func (a assessment) betterThan(other assessment) bool {
	if a.confidence != other.confidence {
		return a.confidence > other.confidence
	}
	if a.year != other.year {
		return a.year > other.year
	}
	if a.rank != other.rank {
		return a.rank < other.rank
	}
	return a.votes > other.votes
}

// SelectCandidate returns the index of the candidate that best describes the
// named film, or -1 when none of them clears the confidence floor.
func SelectCandidate(searchName, searchYear string, candidates []Candidate) int {
	best, bestIndex := assessment{}, -1
	for index, candidate := range candidates {
		current := assess(searchName, searchYear, candidate)
		if current.confidence == confidenceNone {
			continue
		}
		if bestIndex == -1 || current.betterThan(best) {
			best, bestIndex = current, index
		}
	}
	return bestIndex
}

func assess(searchName, searchYear string, candidate Candidate) assessment {
	year := compareYears(searchYear, candidate.ReleaseYear)
	result := assessment{rank: candidate.Rank, votes: candidate.VoteCount, year: year}

	wanted := normalizeTitle(searchName)
	if wanted == "" || year == yearConflict {
		return result
	}

	for _, alternative := range []string{candidate.OriginalTitle, candidate.Title} {
		found := normalizeTitle(alternative)
		if found == "" {
			continue
		}
		if found == wanted {
			result.confidence = confidenceStrong
			return result
		}
	}

	if year == yearExact || year == yearAdjacent {
		for _, alternative := range []string{candidate.OriginalTitle, candidate.Title} {
			if titlesNearlyEqual(wanted, normalizeTitle(alternative)) {
				result.confidence = confidenceFuzzy
				return result
			}
		}
	}

	if year == yearExact && candidate.Rank == 0 {
		result.confidence = confidenceWeak
	}
	return result
}

func compareYears(searchYear, releaseYear string) yearAgreement {
	wanted, wantedErr := strconv.Atoi(strings.TrimSpace(searchYear))
	found, foundErr := strconv.Atoi(strings.TrimSpace(releaseYear))
	if wantedErr != nil || foundErr != nil {
		return yearUnknown
	}
	switch difference := wanted - found; difference {
	case 0:
		return yearExact
	case 1, -1:
		return yearAdjacent
	default:
		return yearConflict
	}
}

// titlesNearlyEqual reports whether two normalised titles describe the same film
// allowing for abbreviation or a differing word.
func titlesNearlyEqual(wanted, found string) bool {
	if wanted == "" || found == "" {
		return false
	}
	shorter, longer := wanted, found
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	if len(shorter) >= minPrefixTitleLength && strings.HasPrefix(longer, shorter) {
		return true
	}
	return tokenSimilarity(wanted, found) >= minTokenSimilarity
}

// tokenSimilarity is the Dice coefficient over the two titles' word sets.
func tokenSimilarity(wanted, found string) float64 {
	wantedTokens := strings.Fields(wanted)
	foundTokens := strings.Fields(found)
	if len(wantedTokens) == 0 || len(foundTokens) == 0 {
		return 0
	}
	wantedSet := make(map[string]struct{}, len(wantedTokens))
	for _, token := range wantedTokens {
		wantedSet[token] = struct{}{}
	}
	foundSet := make(map[string]struct{}, len(foundTokens))
	for _, token := range foundTokens {
		foundSet[token] = struct{}{}
	}
	shared := 0
	for token := range foundSet {
		if _, ok := wantedSet[token]; ok {
			shared++
		}
	}
	return 2 * float64(shared) / float64(len(wantedSet)+len(foundSet))
}

// normalizeTitle folds case and reduces every non-alphanumeric rune to a space.
//
// Reducing punctuation rather than mapping it is what makes typographic
// variation harmless: a tracker writing "The Devil’s Mouth" with U+2019 and TMDB
// writing "The Devil's Mouth" with an ASCII apostrophe both become
// "the devil s mouth", and "Spider-Man" matches "Spider Man".
func normalizeTitle(title string) string {
	var builder strings.Builder
	builder.Grow(len(title))
	spaced := true
	for _, character := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case unicode.IsLetter(character) || unicode.IsDigit(character):
			builder.WriteRune(character)
			spaced = false
		case !spaced:
			builder.WriteByte(' ')
			spaced = true
		}
	}
	return strings.TrimSpace(builder.String())
}

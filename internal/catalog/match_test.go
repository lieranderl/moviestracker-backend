package catalog

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatcherCheckedInCorpus(t *testing.T) {
	type corpusCase struct {
		Name       string      `json:"name"`
		SearchName string      `json:"search_name"`
		SearchYear string      `json:"search_year"`
		Candidates []Candidate `json:"candidates"`
		WantIndex  int         `json:"want_index"`
	}
	raw, err := os.ReadFile("testdata/matcher_corpus.json")
	require.NoError(t, err)
	var corpus []corpusCase
	require.NoError(t, json.Unmarshal(raw, &corpus))
	for _, test := range corpus {
		t.Run(test.Name, func(t *testing.T) {
			require.Equal(t, test.WantIndex, SelectCandidate(test.SearchName, test.SearchYear, test.Candidates))
		})
	}
}

// These cases are the real TMDB responses for releases the previous matcher
// discarded. They were captured by querying TMDB with the scraped name and year
// exactly as the catalog job does, so each one is a regression test for an
// observed production miss rather than a hypothetical.
func TestSelectCandidateRecoversObservedProductionMisses(t *testing.T) {
	tests := []struct {
		name       string
		searchName string
		searchYear string
		candidates []Candidate
		wantIndex  int
		reason     string
	}{
		{
			name:       "prefers the year-exact release over a same-titled adjacent year",
			searchName: "The Cut",
			searchYear: "2024",
			candidates: []Candidate{
				{OriginalTitle: "The Cut", Title: "Вес победы", ReleaseYear: "2025", Rank: 0},
				{OriginalTitle: "The Cut", Title: "The Cut", ReleaseYear: "2024", Rank: 1},
			},
			wantIndex: 1,
			reason:    "both titles match exactly, so the year decides; the old code only ever read index 0",
		},
		{
			name:       "accepts a one-year release drift",
			searchName: "Relay",
			searchYear: "2024",
			candidates: []Candidate{
				{OriginalTitle: "Relay", Title: "Диспетчер", ReleaseYear: "2025", Rank: 0},
			},
			wantIndex: 0,
			reason:    "trackers use the festival year, TMDB records the wide release",
		},
		{
			name:       "accepts a third-language original title on an exact year and top rank",
			searchName: "Magellan",
			searchYear: "2025",
			candidates: []Candidate{
				{OriginalTitle: "Magalhães", Title: "Магеллан", ReleaseYear: "2025", Rank: 0},
			},
			wantIndex: 0,
			reason:    "neither title is comparable; TMDB matched on an alternative title we never see",
		},
		{
			name:       "accepts an abbreviated title",
			searchName: "Mulholland Dr.",
			searchYear: "2001",
			candidates: []Candidate{
				{OriginalTitle: "Mulholland Drive", Title: "Малхолланд Драйв", ReleaseYear: "2001", Rank: 0},
			},
			wantIndex: 0,
			reason:    "\"Mulholland Dr.\" is a prefix of \"Mulholland Drive\" once punctuation is folded",
		},
		{
			name:       "accepts a typographic apostrophe",
			searchName: "The Devil\u2019s Mouth",
			searchYear: "2026",
			candidates: []Candidate{
				{OriginalTitle: "The Devil's Mouth", Title: "Пасть дьявола", ReleaseYear: "2026", Rank: 0},
			},
			wantIndex: 0,
			reason:    "U+2019 against an ASCII apostrophe was a plain string inequality before",
		},
		{
			name:       "prefers the exact title over a longer year-exact title",
			searchName: "Tuner",
			searchYear: "2025",
			candidates: []Candidate{
				{OriginalTitle: "Tuner", Title: "Настройщик", ReleaseYear: "2026", Rank: 0},
				{OriginalTitle: "The Piano Tuner", Title: "The Piano Tuner", ReleaseYear: "2025", Rank: 1},
			},
			wantIndex: 0,
			reason:    "an exact title with one year of drift beats a different film released that year",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := SelectCandidate(test.searchName, test.searchYear, test.candidates)
			require.Equalf(t, test.wantIndex, got, "reason: %s", test.reason)
		})
	}
}

func TestSelectCandidateRejectsUnrelatedResults(t *testing.T) {
	tests := []struct {
		name       string
		searchName string
		searchYear string
		candidates []Candidate
	}{
		{
			name:       "different film in a distant year",
			searchName: "Bad Boys",
			searchYear: "1995",
			candidates: []Candidate{{OriginalTitle: "Different", Title: "Different", ReleaseYear: "2024", Rank: 0}},
		},
		{
			name:       "same title but two years out",
			searchName: "Heat",
			searchYear: "1995",
			candidates: []Candidate{{OriginalTitle: "Heat", Title: "Heat", ReleaseYear: "1998", Rank: 0}},
		},
		{
			name:       "unrelated title on an exact year that TMDB did not rank first",
			searchName: "Nosferatu",
			searchYear: "2024",
			candidates: []Candidate{
				{OriginalTitle: "Something Else", Title: "Something Else", ReleaseYear: "2019", Rank: 0},
				{OriginalTitle: "Unrelated Film", Title: "Unrelated Film", ReleaseYear: "2024", Rank: 1},
			},
		},
		{
			name:       "short common prefix must not match a longer title",
			searchName: "The Cut",
			searchYear: "2024",
			candidates: []Candidate{{OriginalTitle: "The Cutting Edge", Title: "The Cutting Edge", ReleaseYear: "2024", Rank: 1}},
		},
		{
			name:       "no candidates",
			searchName: "Anything",
			searchYear: "2024",
			candidates: nil,
		},
		{
			name:       "empty search name",
			searchName: "",
			searchYear: "2024",
			candidates: []Candidate{{OriginalTitle: "Whatever", Title: "Whatever", ReleaseYear: "2019", Rank: 1}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, -1, SelectCandidate(test.searchName, test.searchYear, test.candidates))
		})
	}
}

// A conflicting year must veto the match even when the titles are identical,
// otherwise remakes collapse onto their originals.
func TestSelectCandidateTreatsYearConflictAsDisqualifying(t *testing.T) {
	candidates := []Candidate{
		{OriginalTitle: "Dune", Title: "Dune", ReleaseYear: "1984", Rank: 0},
		{OriginalTitle: "Dune", Title: "Dune", ReleaseYear: "2021", Rank: 1},
	}
	require.Equal(t, 1, SelectCandidate("Dune", "2021", candidates))
	require.Equal(t, 0, SelectCandidate("Dune", "1984", candidates))
}

// When the tracker gives no year an exact title is still enough, matching the
// previous behaviour, but an uninformative title is not.
func TestSelectCandidateWithoutSearchYear(t *testing.T) {
	require.Equal(t, 0, SelectCandidate("Bad Boys", "", []Candidate{
		{OriginalTitle: "Bad Boys", Title: "Плохие парни", ReleaseYear: "1995", Rank: 0},
	}))
	require.Equal(t, -1, SelectCandidate("Bad Boys", "", []Candidate{
		{OriginalTitle: "Unrelated", Title: "Unrelated", ReleaseYear: "1995", Rank: 0},
	}))
}

func TestSelectCandidatePrefersMoreVotedOfEquallyGoodMatches(t *testing.T) {
	require.Equal(t, 1, SelectCandidate("Carmen", "2022", []Candidate{
		{OriginalTitle: "Carmen", Title: "Carmen", ReleaseYear: "2022", Rank: 5, VoteCount: 10},
		{OriginalTitle: "Carmen", Title: "Carmen", ReleaseYear: "2022", Rank: 5, VoteCount: 900},
	}))
}

func TestNormalizeTitleFoldsCaseAndPunctuation(t *testing.T) {
	for _, test := range []struct{ in, want string }{
		{"The Devil\u2019s Mouth", "the devil s mouth"},
		{"The Devil's Mouth", "the devil s mouth"},
		{"Mulholland Dr.", "mulholland dr"},
		{"Spider-Man: No Way Home", "spider man no way home"},
		{"  WALL·E  ", "wall e"},
		{"Amélie", "amélie"},
		{"12 Angry Men", "12 angry men"},
		{"...", ""},
		{"", ""},
	} {
		t.Run(test.in, func(t *testing.T) {
			require.Equal(t, test.want, normalizeTitle(test.in))
		})
	}
}

func TestTokenSimilarity(t *testing.T) {
	require.InDelta(t, 1.0, tokenSimilarity("the big sleep", "the big sleep"), 0.001)
	// Dice over word sets: 2*3 shared / (4 + 3) tokens.
	require.InDelta(t, 6.0/7.0, tokenSimilarity("the big red sleep", "the big sleep"), 0.001)
	require.InDelta(t, 0.0, tokenSimilarity("alpha", "beta"), 0.001)
	require.InDelta(t, 0.0, tokenSimilarity("", "beta"), 0.001)
}

func TestCompareYears(t *testing.T) {
	require.Equal(t, yearExact, compareYears("2024", "2024"))
	require.Equal(t, yearAdjacent, compareYears("2024", "2025"))
	require.Equal(t, yearAdjacent, compareYears("2025", "2024"))
	require.Equal(t, yearConflict, compareYears("2024", "2026"))
	require.Equal(t, yearUnknown, compareYears("", "2024"))
	require.Equal(t, yearUnknown, compareYears("2024", ""))
}

func TestReleaseYear(t *testing.T) {
	require.Equal(t, "2024", releaseYear("2024-02-12"))
	require.Equal(t, "2024", releaseYear("2024"))
	require.Equal(t, "", releaseYear("202"))
	require.Equal(t, "", releaseYear(""))
}

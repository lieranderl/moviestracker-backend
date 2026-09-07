package main

import (
	"testing"
	"time"

	"github.com/lieranderl/moviestracker-backend/internal/catalog"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigSelectsCollectionURLsAndDefaults(t *testing.T) {
	env := map[string]string{
		"GCP_PROJECT":      "moviestracker-f07e2",
		"RUTOR_HDR10_URLS": " https://one.test ,https://two.test ",
		"TMDBAPIKEY":       "key",
	}
	config, err := loadConfig([]string{"hdr10movies"}, func(key string) string { return env[key] })

	require.NoError(t, err)
	require.Equal(t, catalog.CollectionHDR10, config.collection)
	require.Equal(t, []string{"https://one.test", "https://two.test"}, config.urls)
	require.Equal(t, "moviestracker", config.database)
	require.Equal(t, 10*time.Minute, config.timeout)
	require.Equal(t, 10, config.enricherConcurrency)
}

func TestLoadConfigValidatesEnricherConcurrency(t *testing.T) {
	env := map[string]string{
		"GCP_PROJECT": "moviestracker-f07e2", "RUTOR_URLS": "https://one.test", "TMDBAPIKEY": "key",
		"ENRICHER_CONCURRENCY": "21",
	}
	_, err := loadConfig([]string{"latesttorrentsmovies"}, func(key string) string { return env[key] })
	require.ErrorContains(t, err, "between 1 and 20")

	env["ENRICHER_CONCURRENCY"] = "14"
	config, err := loadConfig([]string{"latesttorrentsmovies"}, func(key string) string { return env[key] })
	require.NoError(t, err)
	require.Equal(t, 14, config.enricherConcurrency)
}

func TestLoadConfigRejectsLegacyDatabaseArgument(t *testing.T) {
	_, err := loadConfig([]string{"latesttorrentsmovies", "mongo"}, func(string) string { return "" })
	require.ErrorContains(t, err, "usage")
}

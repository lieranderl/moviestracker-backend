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
}

func TestLoadConfigRejectsLegacyDatabaseArgument(t *testing.T) {
	_, err := loadConfig([]string{"latesttorrentsmovies", "mongo"}, func(string) string { return "" })
	require.ErrorContains(t, err, "usage")
}

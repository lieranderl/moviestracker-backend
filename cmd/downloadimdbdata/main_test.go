package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadConfigDefaults(t *testing.T) {
	env := map[string]string{"IMDB_BUCKET": "bucket"}
	config, err := loadConfig(func(key string) string { return env[key] })

	require.NoError(t, err)
	require.Equal(t, "https://datasets.imdbws.com/title.ratings.tsv.gz", config.sourceURL)
	require.Equal(t, "title.ratings.tsv.gz", config.object)
	require.Equal(t, 5*time.Minute, config.timeout)
}

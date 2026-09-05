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
	require.Equal(t, "title.ratings.tsv.gz", config.object)
	require.Equal(t, 15*time.Minute, config.refreshInterval)
	require.Equal(t, "8080", config.port)
}

func TestLoadConfigRequiresBucket(t *testing.T) {
	_, err := loadConfig(func(string) string { return "" })
	require.ErrorContains(t, err, "IMDB_BUCKET")
}

package main

import (
	"context"
	"io"
	"net/http"
	"strings"
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

func TestNotifyRefreshPostsInternalEndpoint(t *testing.T) {
	called := false
	client := &http.Client{Transport: refreshRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		called = true
		require.Equal(t, http.MethodPost, request.Method)
		require.Equal(t, "/internal/refresh", request.URL.Path)
		return &http.Response{StatusCode: http.StatusNoContent, Status: "204 No Content", Body: io.NopCloser(strings.NewReader(""))}, nil
	})}

	require.NoError(t, notifyRefresh(context.Background(), client, "https://imdb.test/"))
	require.True(t, called)
}

func TestNotifyRefreshRejectsNonSuccess(t *testing.T) {
	client := &http.Client{Transport: refreshRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Status: "503 Service Unavailable", Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
	})}

	err := notifyRefresh(context.Background(), client, "https://imdb.test")
	require.ErrorContains(t, err, "503")
}

type refreshRoundTripFunc func(*http.Request) (*http.Response, error)

func (function refreshRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

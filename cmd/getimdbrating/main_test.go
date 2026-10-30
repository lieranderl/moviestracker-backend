package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/api/idtoken"
)

const downloader = "movies-imdb-downloader@example.iam.gserviceaccount.com"

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

func TestLoadConfigReadsRefreshInvoker(t *testing.T) {
	env := map[string]string{"IMDB_BUCKET": "bucket", "IMDB_REFRESH_INVOKER": " " + downloader + " "}
	config, err := loadConfig(func(key string) string { return env[key] })

	require.NoError(t, err)
	require.Equal(t, downloader, config.refreshInvoker)
}

func TestRequireInvoker(t *testing.T) {
	validToken := func(email string, verified bool) tokenValidator {
		return func(_ context.Context, token, audience string) (*idtoken.Payload, error) {
			if token != "good" {
				return nil, errors.New("bad signature")
			}
			if audience != "https://imdb.test" {
				return nil, errors.New("wrong audience")
			}
			return &idtoken.Payload{Claims: map[string]any{"email": email, "email_verified": verified}}, nil
		}
	}
	tests := []struct {
		name          string
		authorization string
		validate      tokenValidator
		want          int
	}{
		{"missing token", "", validToken(downloader, true), http.StatusUnauthorized},
		{"not bearer", "Basic good", validToken(downloader, true), http.StatusUnauthorized},
		{"invalid token", "Bearer forged", validToken(downloader, true), http.StatusUnauthorized},
		{"other identity", "Bearer good", validToken("attacker@example.com", true), http.StatusForbidden},
		{"unverified email", "Bearer good", validToken(downloader, false), http.StatusForbidden},
		{"downloader", "Bearer good", validToken(downloader, true), http.StatusNoContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodPost, "https://imdb.test/internal/refresh", nil)
			if test.authorization != "" {
				request.Header.Set("Authorization", test.authorization)
			}
			recorder := httptest.NewRecorder()

			requireInvoker(next, test.validate, downloader).ServeHTTP(recorder, request)

			require.Equal(t, test.want, recorder.Code)
			require.Equal(t, test.want == http.StatusNoContent, called)
		})
	}
}

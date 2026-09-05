package imdb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandlerPreservesIMDbContract(t *testing.T) {
	index, err := ParseRatings(strings.NewReader("tt0468569\t9.0\t100\n"))
	require.NoError(t, err)
	handler := NewHandler(index.Lookup)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/getimdb?imdb_id=tt0468569", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	require.JSONEq(t, `{"Id":"tt0468569","Rating":"9.0","Votes":"100"}`, recorder.Body.String())
}

func TestHandlerValidationAndNotFoundSemantics(t *testing.T) {
	handler := NewHandler(func(string) (Rating, bool) { return Rating{}, false })
	tests := []struct {
		url        string
		wantStatus int
		wantBody   string
	}{
		{"/getimdb", http.StatusBadRequest, "imdb_id is required"},
		{"/getimdb?imdb_id=bad", http.StatusBadRequest, "invalid imdb_id format"},
		{"/getimdb?imdb_id=tt1", http.StatusNotFound, "imdb rating not found"},
	}
	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.url, nil))
			require.Equal(t, test.wantStatus, recorder.Code)
			require.Contains(t, recorder.Body.String(), test.wantBody)
		})
	}
}

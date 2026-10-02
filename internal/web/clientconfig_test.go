package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"telesrv/internal/clientconfig"
)

func TestClientConfigRouteCoexistsWithHomepage(t *testing.T) {
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	})
	h, err := NewHandler(Config{StickerSets: fakeResolver{}, PublicBaseURL: "https://212.189.31.87:8443", ClientConfig: provider})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, clientconfig.Path, http.StatusOK},
		{http.MethodHead, clientconfig.Path, http.StatusOK},
		{http.MethodPost, clientconfig.Path, http.StatusMethodNotAllowed},
		{http.MethodGet, "/", http.StatusOK},
	} {
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.path, nil))
		if recorder.Code != tc.status {
			t.Fatalf("%s %s: got %d, want %d", tc.method, tc.path, recorder.Code, tc.status)
		}
	}
}

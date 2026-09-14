package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoutes(t *testing.T) {
	handler := New(slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, false)
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/healthz", http.StatusOK},
		{"POST", "/healthz", http.StatusMethodNotAllowed},
		{"GET", "/", http.StatusNotFound},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest(tc.method, tc.path, nil))
			if res.Code != tc.status {
				t.Fatalf("status = %d, want %d", res.Code, tc.status)
			}
			if tc.status == http.StatusOK && res.Header().Get("Content-Type") != "application/json" {
				t.Fatal("expected JSON response")
			}
		})
	}
}

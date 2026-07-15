package http

import (
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDegradedHandlerRejectsAllRoutes verifies that the degraded handler
// responds to every representative route with HTTP 503 and a
// Data_Store-unavailable message, per Requirement 9.4. In degraded mode the
// server must refuse all transaction requests rather than crashing silently.
func TestDegradedHandlerRejectsAllRoutes(t *testing.T) {
	handler := NewDegradedHandler()

	cases := []struct {
		name   string
		method string
		target string
	}{
		{name: "root page", method: nethttp.MethodGet, target: "/"},
		{name: "create transaction", method: nethttp.MethodPost, target: "/transactions"},
		{name: "delete transaction", method: nethttp.MethodDelete, target: "/transactions/1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.target, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != nethttp.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", rec.Code, nethttp.StatusServiceUnavailable)
			}
			if !strings.Contains(rec.Body.String(), "Data_Store is unavailable") {
				t.Fatalf("body does not mention Data_Store unavailability: %q", rec.Body.String())
			}
		})
	}
}

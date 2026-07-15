package http

import (
	nethttp "net/http"
)

// dataStoreUnavailableMessage is the message served to every request while the
// server is in degraded mode. It communicates that the Data_Store is
// unavailable and that manual intervention is required to resolve the failure
// (Requirement 9.4).
const dataStoreUnavailableMessage = "The Data_Store is unavailable. Transaction requests cannot be served until the database failure is resolved by manual intervention."

// NewDegradedHandler returns an http.Handler that responds to every request
// with HTTP 503 (Service Unavailable) and a Data_Store-unavailable message.
//
// It is mounted by the server bootstrap when startup schema verification or
// creation fails: rather than crashing silently, the process keeps listening
// but refuses all transaction requests with a clear error, until the failure is
// resolved by manual intervention (Requirement 9.4).
func NewDegradedHandler() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(nethttp.StatusServiceUnavailable)
		_, _ = w.Write([]byte("<h1>Service Unavailable</h1><p>" + dataStoreUnavailableMessage + "</p>"))
	})
}

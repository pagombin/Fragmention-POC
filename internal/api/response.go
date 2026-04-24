// Package api wires the HTTP and WebSocket surfaces. The top-level entrypoint
// is NewRouter, which returns a chi.Router ready to be mounted under a server.
//
// The response envelope is uniform across every endpoint:
//
//	{ "data": <payload> | null, "error": null | { code, message, details } }
//
// Handlers call WriteJSON or WriteError rather than serializing directly.
package api

import (
	"encoding/json"
	"net/http"
)

// Envelope is the uniform response shape.
type Envelope struct {
	Data  any            `json:"data"`
	Error *ErrorPayload  `json:"error"`
}

// ErrorPayload describes a structured error.
type ErrorPayload struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// WriteJSON serializes `data` inside the envelope at the given status.
// It always emits Content-Type: application/json.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Data: data})
}

// WriteError serializes a structured error inside the envelope.
func WriteError(w http.ResponseWriter, status int, code, msg string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Error: &ErrorPayload{
		Code:    code,
		Message: msg,
		Details: details,
	}})
}

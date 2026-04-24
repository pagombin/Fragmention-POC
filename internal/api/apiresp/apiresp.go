// Package apiresp contains the uniform HTTP response envelope and JSON
// helpers. Both the top-level api package and the api/handlers subpackage
// depend on this package to avoid an import cycle.
package apiresp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Envelope is the uniform response shape.
type Envelope struct {
	Data  any           `json:"data"`
	Error *ErrorPayload `json:"error"`
}

// ErrorPayload describes a structured error.
type ErrorPayload struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// WriteJSON serializes `data` inside the envelope at the given status.
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

// DecodeJSON reads a JSON body with strict field policing so typos surface
// as 400s rather than silent no-ops. Callers should wrap this in a handler
// that maps errors to 400 with a descriptive payload.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("empty request body")
		}
		return err
	}
	if dec.More() {
		return errors.New("multiple JSON values in request body")
	}
	return nil
}

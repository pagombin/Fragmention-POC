package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// decodeJSON reads a JSON body with strict field policing so typos in
// client payloads surface as 400s rather than silent no-ops.
func decodeJSON(r *http.Request, dst any) error {
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

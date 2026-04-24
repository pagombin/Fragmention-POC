package compact

import "encoding/json"

// jsonMarshal provides a nil-safe fallback for persistence paths.
func jsonMarshal(v any) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	return json.Marshal(v)
}

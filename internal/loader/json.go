package loader

import "encoding/json"

// jsonMarshal wraps json.Marshal with a nil-safe fallback so persistence
// paths always store a valid JSON payload (never NULL in params/target).
func jsonMarshal(v any) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	return json.Marshal(v)
}

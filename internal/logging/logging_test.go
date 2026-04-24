package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInit_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	log := Init(Config{Level: "debug", Format: FormatJSON, Output: &buf})
	log.Info().Str("service", "test").Msg("hello")

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
	require.Equal(t, "info", record["level"])
	require.Equal(t, "hello", record["message"])
	require.Equal(t, "test", record["service"])
	require.Contains(t, record, "timestamp")
}

func TestSetLevel_Roundtrip(t *testing.T) {
	var buf bytes.Buffer
	_ = Init(Config{Level: "info", Format: FormatJSON, Output: &buf})
	prev, err := SetLevel("warn")
	require.NoError(t, err)
	require.Equal(t, "info", prev)
	require.Equal(t, "warn", Level())

	_, err = SetLevel("bogus")
	require.Error(t, err)
}

func TestInit_DefaultLevelIsInfo(t *testing.T) {
	var buf bytes.Buffer
	log := Init(Config{Output: &buf})
	// debug below info threshold should be suppressed
	log.Debug().Msg("should not appear")
	log.Info().Msg("should appear")
	require.False(t, strings.Contains(buf.String(), "should not appear"))
	require.True(t, strings.Contains(buf.String(), "should appear"))
}

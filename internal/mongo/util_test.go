package mongo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in            string
		maj, min, pat int
	}{
		{"5.0.9", 5, 0, 9},
		{"7.0.2-rc0", 7, 0, 2},
		{"4.4.18", 4, 4, 18},
		{"", 0, 0, 0},
		{"notaversion", 0, 0, 0},
	}
	for _, c := range cases {
		maj, minr, pat := parseVersion(c.in)
		require.Equal(t, c.maj, maj, "maj %q", c.in)
		require.Equal(t, c.min, minr, "min %q", c.in)
		require.Equal(t, c.pat, pat, "pat %q", c.in)
	}
}

func TestExtractCompressor(t *testing.T) {
	cases := map[string]string{
		"block_compressor=snappy,allocation_size=4KB":  "snappy",
		"access_pattern_hint=none,block_compressor=zstd": "zstd",
		"no_compressor_here":                             "",
	}
	for in, want := range cases {
		require.Equal(t, want, extractCompressor(in), "input=%q", in)
	}
}

func TestClassifyCollection(t *testing.T) {
	require.Equal(t, CollTypeSystem, classifyCollection("admin", "users", nil))
	require.Equal(t, CollTypeSystem, classifyCollection("poc_db_1", "system.views", nil))
	require.Equal(t, CollTypeRegular, classifyCollection("poc_db_1", "users", nil))
}

package deleter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPatternConfig_ValidateRandomByID(t *testing.T) {
	require.NoError(t, PatternConfig{Kind: PatternRandomByID, Ratio: 0.3}.Validate(0.95))
	require.Error(t, PatternConfig{Kind: PatternRandomByID, Ratio: 0}.Validate(0.95))
	require.Error(t, PatternConfig{Kind: PatternRandomByID, Ratio: 0.99}.Validate(0.95))
}

func TestPatternConfig_ValidateTTL(t *testing.T) {
	cutoff := time.Now().Add(-time.Hour)
	require.NoError(t, PatternConfig{Kind: PatternTTLSimulated, Field: "created_at", RangeBefore: &cutoff}.Validate(0.95))
	require.Error(t, PatternConfig{Kind: PatternTTLSimulated}.Validate(0.95)) // no range_before
}

func TestPatternConfig_ValidateRange(t *testing.T) {
	cutoff := time.Now()
	require.NoError(t, PatternConfig{Kind: PatternRangeByField, Field: "ts", RangeBefore: &cutoff}.Validate(0.95))
	require.Error(t, PatternConfig{Kind: PatternRangeByField, RangeBefore: &cutoff}.Validate(0.95)) // no field
}

func TestPatternConfig_ValidateModulo(t *testing.T) {
	require.NoError(t, PatternConfig{Kind: PatternModulo, Ratio: 0.1, Modulus: 10}.Validate(0.95))
	require.Error(t, PatternConfig{Kind: PatternModulo}.Validate(0.95)) // ratio unset
}

func TestPatternConfig_ValidatePrefix(t *testing.T) {
	require.NoError(t, PatternConfig{Kind: PatternPrefixByID, Prefix: "0a"}.Validate(0.95))
	require.Error(t, PatternConfig{Kind: PatternPrefixByID}.Validate(0.95))
}

func TestPatternConfig_UnknownKind(t *testing.T) {
	require.Error(t, PatternConfig{Kind: PatternKind("bogus"), Ratio: 0.1}.Validate(0.95))
}

func TestParams_Validate(t *testing.T) {
	require.NoError(t, Params{BatchSize: 1000, MaxRatio: 0.95}.Validate())
	require.Error(t, Params{BatchSize: 0, MaxRatio: 0.95}.Validate())
	require.Error(t, Params{BatchSize: 100, MaxRatio: 0}.Validate())
	require.Error(t, Params{BatchSize: 100, MaxRatio: 1.0}.Validate())
}

func TestTargetsOverlap(t *testing.T) {
	a := TargetSpec{Entries: []Target{{Database: "d", Collection: "a"}}}
	b := TargetSpec{Entries: []Target{{Database: "d", Collection: "b"}}}
	c := TargetSpec{Entries: []Target{{Database: "d", Collection: "a"}}}
	require.False(t, targetsOverlap(a, b))
	require.True(t, targetsOverlap(a, c))
}

func TestGenerateToken_UniqueAndLong(t *testing.T) {
	t1 := generateToken()
	t2 := generateToken()
	require.Len(t, t1, 64)
	require.Len(t, t2, 64)
	require.NotEqual(t, t1, t2)
}

func TestFNV64_Deterministic(t *testing.T) {
	require.Equal(t, fnv64("hello"), fnv64("hello"))
	require.NotEqual(t, fnv64("hello"), fnv64("world"))
}

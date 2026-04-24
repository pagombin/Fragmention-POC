package loader

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTargetsOverlap(t *testing.T) {
	a := TargetSpec{Entries: []Target{{Database: "poc_db_1", Collection: "coll_a"}, {Database: "poc_db_1", Collection: "coll_b"}}}
	b := TargetSpec{Entries: []Target{{Database: "poc_db_2", Collection: "coll_a"}}}
	c := TargetSpec{Entries: []Target{{Database: "poc_db_1", Collection: "coll_b"}}}
	require.False(t, targetsOverlap(a, b))
	require.True(t, targetsOverlap(a, c))
	require.True(t, targetsOverlap(c, a))
}

func TestParams_Validate(t *testing.T) {
	require.NoError(t, Params{BatchSize: 1000}.Validate())
	require.Error(t, Params{BatchSize: 0}.Validate())
	require.Error(t, Params{BatchSize: 1000, Workers: -1}.Validate())
	require.Error(t, Params{BatchSize: 1000, DocsPerSecond: -1}.Validate())
}

func TestLive_RaceSafetyPattern(t *testing.T) {
	l := newLive(Params{BatchSize: 100, Workers: 4})
	require.Equal(t, 100, l.get().BatchSize)
	l.set(Params{BatchSize: 200, Workers: 8})
	require.Equal(t, 200, l.get().BatchSize)
	require.Equal(t, 8, l.get().Workers)
}

func TestTargetKey(t *testing.T) {
	require.Equal(t, "db.coll", Target{Database: "db", Collection: "coll"}.Key())
}

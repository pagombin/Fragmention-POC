package workload

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPickOp_WeightedMix(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	s := Spec{ReadWeight: 0.7, WriteWeight: 0.2, AggregateWeight: 0.1}
	counts := map[OpType]int{}
	for i := 0; i < 10000; i++ {
		counts[pickOp(rng, s)]++
	}
	require.InDelta(t, 7000, counts[OpRead], 500)
	require.InDelta(t, 2000, counts[OpWrite], 300)
	require.InDelta(t, 1000, counts[OpAggregate], 300)
}

func TestParams_Validate(t *testing.T) {
	require.NoError(t, Params{TargetOpsPerSec: 100, Workers: 4}.Validate())
	require.Error(t, Params{TargetOpsPerSec: -1, Workers: 4}.Validate())
	require.Error(t, Params{TargetOpsPerSec: 100, Workers: -1}.Validate())
}

func TestBuildLimiter_NilWhenUnlimited(t *testing.T) {
	require.Nil(t, buildLimiter(Params{TargetOpsPerSec: 0}))
	require.NotNil(t, buildLimiter(Params{TargetOpsPerSec: 50}))
}

func TestReasonFor(t *testing.T) {
	require.Equal(t, "none", reasonFor(nil))
	require.Equal(t, "mongo", reasonFor(errExample))
}

var errExample = testErr("boom")

type testErr string

func (e testErr) Error() string { return string(e) }

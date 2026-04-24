package compact

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIsSystemDB(t *testing.T) {
	require.True(t, isSystemDB("admin"))
	require.True(t, isSystemDB("config"))
	require.True(t, isSystemDB("local"))
	require.False(t, isSystemDB("poc_db_1"))
}

func TestParams_Validate(t *testing.T) {
	require.NoError(t, Params{MaxReplicationLag: 10 * time.Second, StepdownWaitTimeout: 60 * time.Second}.Validate())
	require.Error(t, Params{StepdownWaitTimeout: 0}.Validate())
	require.Error(t, Params{MaxReplicationLag: -1, StepdownWaitTimeout: time.Second}.Validate())
}

func TestEstimateDuration(t *testing.T) {
	require.Equal(t, 30*time.Second, estimateDuration(1))
	require.Equal(t, 10*30*time.Second, estimateDuration(10))
}

func TestCollectionRefKey(t *testing.T) {
	require.Equal(t, "db.coll", CollectionRef{Database: "db", Collection: "coll"}.Key())
}

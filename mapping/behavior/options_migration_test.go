package behavior

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKindsOptionMigrationBehavior(t *testing.T) {
	o := options{}
	WithKinds()(&o)
	require.Nil(t, o.kinds)
	require.Zero(t, o.kindsMask)
	set := []Kind{OutputOnly, Identifier}
	WithKinds(set...)(&o)
	require.Equal(t, set, o.kinds)
	require.NotZero(t, o.kindsMask)
	set[0] = InputOnly
	require.Equal(t, OutputOnly, o.kinds[0])
	WithKinds()(&o)
	require.Nil(t, o.kinds)
	require.Zero(t, o.kindsMask)
}

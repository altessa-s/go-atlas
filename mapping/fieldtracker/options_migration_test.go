package fieldtracker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIgnoreFieldsOptionMigrationBehavior(t *testing.T) {
	o := defaultOptions()
	WithIgnoreFields()(o)
	require.Empty(t, o.ignoreFields)
	WithIgnoreFields("a", "a", " ")(o)
	require.Equal(t, map[string]struct{}{"a": {}, " ": {}}, o.ignoreFields)
	fields := []string{"before"}
	option := WithIgnoreFields(fields...)
	fields[0] = "after"
	option(o)
	require.Contains(t, o.ignoreFields, "after")
	require.NotContains(t, o.ignoreFields, "before")
}

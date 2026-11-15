package saga

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCallbackOptionsMigrationBehavior(t *testing.T) {
	o := &options{onDeadLetter: func(*Instance) {}, shouldRetry: func(error) bool { return false }}
	WithOnDeadLetter(nil)(o)
	WithShouldRetry(nil)(o)
	require.Nil(t, o.onDeadLetter)
	require.Nil(t, o.shouldRetry)
	fn := func(error) bool { return true }
	WithShouldRetry(fn)(o)
	require.True(t, o.shouldRetry(errors.New("x")))
}

func TestCompensationPolicyOptionMigrationBehavior(t *testing.T) {
	cfg := buildOptions(WithCompensationPolicy(Policy(99)))
	require.Equal(t, Policy(99), cfg.policy)
}

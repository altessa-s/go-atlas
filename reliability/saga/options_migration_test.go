package saga

import (
	"errors"
	"testing"
	"time"

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

func TestRetentionOptionDefaultAndDisable(t *testing.T) {
	require.Equal(t, DefaultRetention, newOptions().retention)
	require.Equal(t, 30*24*time.Hour, DefaultRetention)
	require.Zero(t, newOptions(WithRetention(0)).retention)
}

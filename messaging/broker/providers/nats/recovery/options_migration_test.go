package recovery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCallbackOptionsMigrationBehavior(t *testing.T) {
	o := &options{}
	WithOnRecoverySuccess(nil)(o)
	WithOnRecoveryFailure(nil)(o)
	WithOnManualRecoveryNeeded(nil)(o)
	WithOnStaleRecoveryCleared(nil)(o)
	require.Nil(t, o.onRecoverySuccess)
	require.Nil(t, o.onRecoveryFailure)
	require.Nil(t, o.onManualRecoveryNeeded)
	require.Nil(t, o.onStaleRecoveryCleared)
	WithOnRecoverySuccess(RecoverySuccessCallback(func(string, string) {}))(o)
	require.NotNil(t, o.onRecoverySuccess)
}

func TestRecoveryStrategyOptionMigrationBehavior(t *testing.T) {
	o := newStreamOptions(WithRecoveryStrategy(RecoveryStrategy(99)))
	require.Equal(t, RecoveryStrategy(99), o.strategy)
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dispatchconfig "github.com/altessa-s/go-atlas/config/dispatch"
	natsconfig "github.com/altessa-s/go-atlas/config/nats"
	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// ozzo-validation skips every rule but Required when the value is the zero
// value of its type, so validation.Min(N) with N > 0 silently accepts 0. Each
// case below zeroes exactly one field of an otherwise valid configuration and
// asserts that Validate rejects it — the assertion that fails if a
// validation.Required is ever dropped from a Min-guarded field.
func TestValidate_ZeroRejectedByMinGuardedFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		validate func() error
	}{
		{"Saga.MaxStepAttempts", func() error {
			c := validSaga()
			c.MaxStepAttempts = 0
			return c.Validate()
		}},
		{"Saga.MaxCompensationAttempts", func() error {
			c := validSaga()
			c.MaxCompensationAttempts = 0
			return c.Validate()
		}},
		{"Saga.RecoveryBatchSize", func() error {
			c := validSaga()
			c.RecoveryBatchSize = 0
			return c.Validate()
		}},
		{"Nats.MaxPingsOut", func() error {
			c := natsconfig.Default()
			c.MaxPingsOut = 0
			return c.Validate()
		}},
		{"WAL.MaxSegmentBytes", func() error {
			c := validWAL(t)
			c.MaxSegmentBytes = 0
			return c.Validate()
		}},
		{"WAL.MaxBytes", func() error {
			c := validWAL(t)
			c.MaxBytes = 0
			return c.Validate()
		}},
		{"Health.AdaptiveBufferThreshold", func() error {
			c := observabilityconfig.DefaultHealth()
			c.AdaptiveBufferThreshold = 0
			return c.Validate()
		}},
		{"Health.AdaptiveBufferMultiplier", func() error {
			c := observabilityconfig.DefaultHealth()
			c.AdaptiveBufferMultiplier = 0
			return c.Validate()
		}},
		{"Health.MaxAdaptiveBuffer", func() error {
			c := observabilityconfig.DefaultHealth()
			c.MaxAdaptiveBuffer = 0
			return c.Validate()
		}},
		{"NatsConsumer.MaxAckPending", func() error {
			c := validNatsConsumer()
			c.MaxAckPending = 0
			return c.Validate()
		}},
		{"NatsConsumer.MaxWaiting", func() error {
			c := validNatsConsumer()
			c.MaxWaiting = 0
			return c.Validate()
		}},
		{"StorageNATSConfig.Replicas", func() error {
			c := storageconfig.NATSConfig{Bucket: "bucket", Replicas: 0}
			return c.Validate()
		}},
		{"NatsRecovery.MaxRecoveryAttempts", func() error {
			c := natsconfig.DefaultRecovery()
			c.MaxRecoveryAttempts = 0
			return c.Validate()
		}},
		{"ProbabilisticFilterBloomDefaults.FalsePositiveRate", func() error {
			c := probfilterconfig.NewBloomDefaults()
			c.FalsePositiveRate = 0
			return c.Validate()
		}},
		{"ProbabilisticFilterCuckooDefaults.CapacityMultiplier", func() error {
			c := probfilterconfig.NewCuckooDefaults()
			c.CapacityMultiplier = 0
			return c.Validate()
		}},
		{"ProbabilisticFilterCuckooDefaults.MaxCapacity", func() error {
			c := probfilterconfig.NewCuckooDefaults()
			c.MaxCapacity = 0
			return c.Validate()
		}},
		{"ProbabilisticFilterBloomConfig.FalsePositiveRate", func() error {
			rate := 0.0
			c := probfilterconfig.BloomConfig{ExpectedItems: 1000, FalsePositiveRate: &rate}
			return c.Validate()
		}},
		{"ProbabilisticFilterCuckooConfig.CapacityMultiplier", func() error {
			mult := 0.0
			c := probfilterconfig.CuckooConfig{Capacity: 1000, CapacityMultiplier: &mult}
			return c.Validate()
		}},
		{"ProbabilisticFilterCuckooConfig.MaxCapacity", func() error {
			maxCap := int64(0)
			c := probfilterconfig.CuckooConfig{Capacity: 1000, MaxCapacity: &maxCap}
			return c.Validate()
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Error(t, tc.validate(), "zero value must be rejected")
		})
	}
}

// The valid* helpers must themselves validate, otherwise a case above could
// pass for the wrong reason.
func TestValidate_ZeroRejectedByMinGuardedFields_BaselinesAreValid(t *testing.T) {
	t.Parallel()

	baselines := map[string]func() error{
		"Saga":              func() error { c := validSaga(); return c.Validate() },
		"Nats":              func() error { c := natsconfig.Default(); return c.Validate() },
		"WAL":               func() error { c := validWAL(t); return c.Validate() },
		"Health":            func() error { c := observabilityconfig.DefaultHealth(); return c.Validate() },
		"NatsConsumer":      func() error { c := validNatsConsumer(); return c.Validate() },
		"StorageNATSConfig": func() error { c := storageconfig.NATSConfig{Bucket: "bucket", Replicas: 3}; return c.Validate() },
		"NatsRecovery":      func() error { c := natsconfig.DefaultRecovery(); return c.Validate() },
		"BloomDefaults":     func() error { c := probfilterconfig.NewBloomDefaults(); return c.Validate() },
		"CuckooDefaults":    func() error { c := probfilterconfig.NewCuckooDefaults(); return c.Validate() },
	}

	for name, validate := range baselines {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validate())
		})
	}
}

// Pins the ozzo-validation behavior the fixes above compensate for: Min(1)
// alone accepts 0, and only the added Required rejects it. If a future
// ozzo-validation upgrade changes this, the Required rules become redundant
// and this test says so.
func TestValidate_MinAloneAcceptsZero(t *testing.T) {
	t.Parallel()

	require.NoError(t, validation.Validate(0, validation.Min(1)))
	require.Error(t, validation.Validate(0, validation.Required, validation.Min(1)))
	require.NoError(t, validation.Validate(time.Duration(0), validation.Min(time.Millisecond)))
	require.Error(t, validation.Validate(time.Duration(0), ozzo_rules.Duration(), validation.Min(time.Millisecond)))
}

// validation.Min(time.Duration(0)) is not a no-op: the zero value is skipped,
// but a negative duration is still rejected. Removing those rules as
// "vacuously true" would let negative durations through.
func TestValidate_MinZeroDurationRejectsNegative(t *testing.T) {
	t.Parallel()

	require.NoError(t, validation.Validate(time.Duration(0), validation.Min(time.Duration(0))))
	require.Error(t, validation.Validate(-time.Second, validation.Min(time.Duration(0))))
}

func validSaga() sagaconfig.Config {
	c := sagaconfig.Default()
	c.Storage = sagaconfig.DefaultStorage()

	return c
}

func validWAL(t *testing.T) dispatchconfig.WAL {
	t.Helper()

	return dispatchconfig.WAL{
		Enabled:         true,
		Dir:             t.TempDir(),
		MaxSegmentBytes: 64 << 20,
		MaxBytes:        1 << 30,
		FsyncInterval:   5 * time.Millisecond,
	}
}

func validNatsConsumer() natsconfig.Consumer {
	return natsconfig.Consumer{
		DeliverPolicy: natsconfig.DeliverPolicyAll,
		AckPolicy:     natsconfig.AckPolicyExplicit,
		AckWait:       30 * time.Second,
		MaxDeliver:    -1,
		ReplayPolicy:  natsconfig.ReplayPolicyInstant,
		MaxAckPending: 1000,
		MaxWaiting:    512,
	}
}

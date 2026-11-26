// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxconfig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	outboxconfig "github.com/altessa-s/go-atlas/config/outbox"
)

// See config/validate_min_test.go: each case zeroes one Min-guarded field of
// an otherwise valid configuration and asserts that Validate rejects it.
func TestValidate_ZeroRejectedByMinGuardedFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		validate func() error
	}{
		{"Outbox.FetchTimeout", func() error {
			c := outboxconfig.Default()
			c.FetchTimeout = 0
			return c.Validate()
		}},
		{"Outbox.HandleTimeout", func() error {
			c := outboxconfig.Default()
			c.HandleTimeout = 0
			return c.Validate()
		}},
		{"Outbox.UpdateTimeout", func() error {
			c := outboxconfig.Default()
			c.UpdateTimeout = 0
			return c.Validate()
		}},
		{"Outbox.MessagesBatchSize", func() error {
			c := outboxconfig.Default()
			c.MessagesBatchSize = 0
			return c.Validate()
		}},
		{"Outbox.RetryMaxAttempts", func() error {
			c := outboxconfig.Default()
			c.RetryMaxAttempts = 0
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

func TestValidate_ZeroRejectedByMinGuardedFields_BaselinesAreValid(t *testing.T) {
	t.Parallel()

	require.NoError(t, outboxconfig.Default().Validate())
}

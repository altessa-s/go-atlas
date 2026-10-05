// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/secrets/internal/base"
)

// requireForgetDetaches starts a read flight under key that blocks until
// released, runs forget, and checks that a read started afterwards runs on
// its own instead of joining the earlier flight.
func requireForgetDetaches(t *testing.T, group *base.SingleflightGroup, key string, forget func()) {
	t.Helper()

	started, release := make(chan struct{}), make(chan struct{})
	earlier := make(chan string, 1)
	go func() {
		v, _ := base.DoTyped(group, key, func() (string, error) {
			close(started)
			<-release
			return "before write", nil
		})
		earlier <- v
	}()
	<-started

	forget()
	got, err := base.DoTyped(group, key, func() (string, error) { return "after write", nil })
	require.NoError(t, err)
	require.Equal(t, "after write", got, "a read started after the write must not join an earlier flight")

	close(release)
	require.Equal(t, "before write", <-earlier)
}

// TestForgetReads checks that a completed Save or Delete detaches in-flight
// Value and List reads of the key.
func TestForgetReads(t *testing.T) {
	t.Parallel()

	s := &Storage[string]{opts: &options[string]{}}
	t.Run("value", func(t *testing.T) {
		t.Parallel()
		requireForgetDetaches(t, &s.SingleflightGroup, s.valueFlightKey("enc-key"), func() { s.forgetReads("enc-key") })
	})
	t.Run("list", func(t *testing.T) {
		t.Parallel()
		requireForgetDetaches(t, &s.SingleflightGroup, s.listFlightKey(), func() { s.forgetReads("enc-key") })
	})
}

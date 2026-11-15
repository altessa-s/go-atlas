// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzEvent_IsReadyForRetry(f *testing.F) {
	f.Add(uint32(0), uint32(10))
	f.Add(uint32(5), uint32(10))
	f.Add(uint32(10), uint32(10))

	f.Fuzz(func(t *testing.T, attempts, maxAttempts uint32) {
		e := &Event{Attempts: attempts}
		got := e.isReadyForRetry(maxAttempts)
		want := attempts < maxAttempts
		assert.Equal(t, want, got)
	})
}

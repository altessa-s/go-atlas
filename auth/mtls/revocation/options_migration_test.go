// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation

import (
	"testing"
	"time"
)

func TestOptionsMigrationBaseline(t *testing.T) {
	t.Run("fail mode", func(t *testing.T) {
		got := defaultOptions()
		WithFailMode(FailMode(99))(got)
		if got.failMode != DefaultFailMode {
			t.Fatalf("invalid mode changed default: %v", got.failMode)
		}
		WithFailMode(FailClosed)(got)
		if got.failMode != FailClosed {
			t.Fatalf("valid mode not applied: %v", got.failMode)
		}
		WithFailMode(FailMode(99))(got)
		if got.failMode != FailClosed {
			t.Fatalf("invalid mode replaced configured value: %v", got.failMode)
		}
	})
	t.Run("clock", func(t *testing.T) {
		got := defaultOptions()
		fn := func() time.Time { return time.Unix(1, 0) }
		WithClock(fn)(got)
		if got.now == nil || got.now() != fn() {
			t.Fatal("clock not applied")
		}
		WithClock(nil)(got)
		if got.now == nil || got.now() != fn() {
			t.Fatal("nil clock cleared configured value")
		}
	})
}

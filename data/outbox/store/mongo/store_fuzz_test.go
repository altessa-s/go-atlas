// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzWithCollectionName(f *testing.F) {
	f.Add("events")
	f.Add("")
	f.Add("  spaced  ")
	f.Add("events_outbox")

	f.Fuzz(func(t *testing.T, name string) {
		opts := newOptions(WithCollectionName(name))
		assert.NotNil(t, opts, "newOptions returned nil")
	})
}

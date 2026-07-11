// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExposeEntityID_DefaultOff pins the secure default: the stored entity id is
// not echoed to a duplicate caller unless explicitly enabled, since that caller
// is not verified to be the principal that created the original entity.
func TestExposeEntityID_DefaultOff(t *testing.T) {
	t.Parallel()

	require.False(t, newOptions().exposeEntityID, "entity id exposure must be off by default")
	require.True(t, newOptions(WithExposeEntityID()).exposeEntityID, "WithExposeEntityID must opt in")
}

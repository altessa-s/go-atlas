// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConstants(t *testing.T) {
	require.NotEqual(t, "", DefaultFallbackMessage)
	require.NotEqual(t, "", ContentTypePlainText)
	require.NotEqual(t, "", DefaultCodecJSON)
}

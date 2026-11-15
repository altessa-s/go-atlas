// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retryconfig_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	retryconfig "github.com/altessa-s/go-atlas/config/retry"
)

func TestValidateNegativeElapsedTime(t *testing.T) {
	t.Parallel()
	cfg := retryconfig.Default()
	cfg.MaxElapsedTime = -time.Second
	require.Error(t, cfg.Validate())
}

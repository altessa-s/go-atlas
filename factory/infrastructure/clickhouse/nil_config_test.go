// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/infrastructure/clickhouse"
)

func TestClientOptionsNilConfig(t *testing.T) {
	t.Parallel()
	require.NotPanics(t, func() {
		opts, err := factory.New(nil).ClientOptions()
		require.Nil(t, opts)
		require.Error(t, err)
	})
}

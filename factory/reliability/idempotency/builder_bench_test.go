// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	factory "github.com/altessa-s/go-atlas/factory/reliability/idempotency"
)

func BenchmarkBuildSQL(b *testing.B) {
	db, fake := testhelpers.NewFakeSQL(b, nil)
	fake.DisableRecording()
	cfg := sqlConfig()
	b.ReportAllocs()
	for b.Loop() {
		_, err := factory.New(cfg).UseSQLDB(db).Build()
		require.NoError(b, err)
	}
}

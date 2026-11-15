// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import "testing"

func TestOptionModesMigrationBaseline(t *testing.T) {
	t.Run("schema migration", func(t *testing.T) {
		got := defaultOptions()
		WithSchemaMigration(SchemaMigrationMode("bad"))(got)
		if got.schemaMigrationMode != DefaultSchemaMigrationMode {
			t.Fatal("invalid mode changed default")
		}
		WithSchemaMigration(SchemaMigrationModeAdditive)(got)
		if got.schemaMigrationMode != SchemaMigrationModeAdditive {
			t.Fatal("valid mode not applied")
		}
		WithSchemaMigration(SchemaMigrationMode("bad"))(got)
		if got.schemaMigrationMode != SchemaMigrationModeAdditive {
			t.Fatal("invalid mode replaced configured value")
		}
	})
	t.Run("schema check", func(t *testing.T) {
		got := defaultOptions()
		WithSchemaCheck(SchemaCheckMode("bad"))(got)
		if got.schemaCheckMode != DefaultSchemaCheckMode {
			t.Fatal("invalid mode changed default")
		}
		WithSchemaCheck(SchemaCheckModeDisabled)(got)
		if got.schemaCheckMode != SchemaCheckModeDisabled {
			t.Fatal("valid mode not applied")
		}
		WithSchemaCheck(SchemaCheckMode("bad"))(got)
		if got.schemaCheckMode != SchemaCheckModeDisabled {
			t.Fatal("invalid mode replaced configured value")
		}
	})
	t.Run("time range", func(t *testing.T) {
		got := defaultOptions()
		WithTimeRangeMode(TimeRangeMode("bad"))(got)
		if got.timeRangeMode != DefaultTimeRangeMode {
			t.Fatal("invalid mode changed default")
		}
		WithTimeRangeMode(TimeRangeModeDisabled)(got)
		if got.timeRangeMode != TimeRangeModeDisabled {
			t.Fatal("valid mode not applied")
		}
		WithTimeRangeMode(TimeRangeMode("bad"))(got)
		if got.timeRangeMode != TimeRangeModeDisabled {
			t.Fatal("invalid mode replaced configured value")
		}
	})
}

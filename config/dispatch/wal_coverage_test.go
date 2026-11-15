// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dispatchconfig_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	dispatchconfig "github.com/altessa-s/go-atlas/config/dispatch"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func TestWAL_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&dispatchconfig.WAL{}).Validate(), "disabled WAL is not validated")

	valid := dispatchconfig.WAL{Enabled: true, Dir: "/var/lib/wal", MaxSegmentBytes: 1, MaxBytes: 1}
	require.NoError(t, valid.Validate())

	err := (&dispatchconfig.WAL{Enabled: true}).Validate()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	require.Contains(t, verrs, "Dir")
	require.Contains(t, verrs, "MaxSegmentBytes")
	require.Contains(t, verrs, "MaxBytes")

	negative := valid
	negative.MaxBytes = -1
	verrs, ok = errors.AsType[validation.Errors](negative.Validate())
	require.True(t, ok)
	require.Contains(t, verrs, "MaxBytes")
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     dispatchconfig.Config
		wantErr bool
	}{
		{name: "nil WAL", cfg: dispatchconfig.Config{}},
		{name: "disabled WAL", cfg: dispatchconfig.Config{WAL: &dispatchconfig.WAL{}}},
		{
			name: "valid enabled WAL",
			cfg: dispatchconfig.Config{
				WAL: &dispatchconfig.WAL{Enabled: true, Dir: "/var/lib/wal", MaxSegmentBytes: 1, MaxBytes: 1},
			},
		},
		{name: "invalid enabled WAL", cfg: dispatchconfig.Config{WAL: &dispatchconfig.WAL{Enabled: true}}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.cfg.Validate()
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			verrs, ok := errors.AsType[validation.Errors](err)
			require.True(t, ok, "want validation.Errors, got %v", err)
			nested, ok := errors.AsType[validation.Errors](verrs["WAL"])
			require.True(t, ok, "want nested WAL errors, got %v", verrs)
			require.Contains(t, nested, "Dir")
		})
	}
}

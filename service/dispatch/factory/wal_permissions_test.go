// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/io/wal"
	"github.com/altessa-s/go-atlas/service/dispatch/factory"

	dispatchconfig "github.com/altessa-s/go-atlas/config/dispatch"
)

func TestBuild_WALPermissionPolicy(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions only")
	}
	for _, insecure := range []bool{false, true} {
		name := "secure"
		if insecure {
			name = "legacy"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			require.NoError(t, os.Chmod(dir, 0o755))
			cfg := &dispatchconfig.Config{BatchSize: 1, Workers: 1, WAL: &dispatchconfig.WAL{Enabled: true, Dir: dir, InsecurePermissions: insecure}}
			engine, err := factory.New[string](cfg).WithSink(&mockSink{}).WithCodec(&mockCodec{}).Build()
			if !insecure {
				require.ErrorIs(t, err, wal.ErrInsecurePermissions)
				return
			}
			require.NoError(t, err)
			require.NoError(t, engine.Shutdown(t.Context()))
		})
	}
}

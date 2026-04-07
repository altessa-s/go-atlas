// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux

package landlock

import (
	"testing"

	"golang.org/x/sys/unix"
)

// TestAccessMasks verifies that [accessMasks] degrades gracefully across
// Landlock ABI versions: the read mask is always the base read+execute set,
// the write mask adds truncate only on ABI 3+, and no unexpected flags leak
// into either mask. This is the regression guard for the documented
// "degrades gracefully on v1 and v2 kernels" contract — a future change
// that rewires accessMasks to assume a newer ABI would break RHEL 9 and
// similarly-aged hosts, and must be caught here before it lands.
func TestAccessMasks(t *testing.T) {
	cases := []struct {
		name      string
		abi       int
		wantRead  uint64
		wantWrite uint64
	}{
		{
			name:      "v1 kernel has no truncate",
			abi:       abiV1,
			wantRead:  readAccessMask,
			wantWrite: writeAccessMask,
		},
		{
			name:      "v2 kernel has no truncate",
			abi:       abiV1 + 1, // v2
			wantRead:  readAccessMask,
			wantWrite: writeAccessMask,
		},
		{
			name:      "v3 kernel gains truncate",
			abi:       abiV3,
			wantRead:  readAccessMask,
			wantWrite: writeAccessMask | unix.LANDLOCK_ACCESS_FS_TRUNCATE,
		},
		{
			name:      "future abi keeps truncate",
			abi:       abiV3 + 3, // v6
			wantRead:  readAccessMask,
			wantWrite: writeAccessMask | unix.LANDLOCK_ACCESS_FS_TRUNCATE,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRead, gotWrite := accessMasks(tc.abi)
			if gotRead != tc.wantRead {
				t.Errorf("read mask: got %#x, want %#x", gotRead, tc.wantRead)
			}
			if gotWrite != tc.wantWrite {
				t.Errorf("write mask: got %#x, want %#x", gotWrite, tc.wantWrite)
			}
		})
	}
}

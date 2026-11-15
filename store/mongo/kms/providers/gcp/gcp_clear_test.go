// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsgcp_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	kmsgcp "github.com/altessa-s/go-atlas/store/mongo/kms/providers/gcp"
)

func TestGoogle_Clear_DropsCachedCredentials(t *testing.T) {
	t.Parallel()

	p := kmsgcp.New("project", "email", "key", "location", "ring", "name")
	require.Len(t, p.Credentials()["gcp"], 2)

	p.Clear()

	require.Empty(t, p.Credentials()["gcp"], "Credentials after Clear must not return the cached map")
}

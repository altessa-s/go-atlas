// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsazure_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	kmsazure "github.com/altessa-s/go-atlas/store/mongo/kms/providers/azure"
)

func TestAzure_Clear_DropsCachedCredentials(t *testing.T) {
	t.Parallel()

	p := kmsazure.New("client", "secret", "tenant", "key")
	require.Len(t, p.Credentials()["azure"], 3)

	p.Clear()

	require.Empty(t, p.Credentials()["azure"], "Credentials after Clear must not return the cached map")
}

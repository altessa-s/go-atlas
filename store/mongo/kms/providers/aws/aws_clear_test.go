// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsaws_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	kmsaws "github.com/altessa-s/go-atlas/store/mongo/kms/providers/aws"
)

func TestAmazon_Clear_DropsCachedCredentials(t *testing.T) {
	t.Parallel()

	p := kmsaws.New("AKID", "SECRET", "arn")
	require.Len(t, p.Credentials()["aws"], 2)

	p.Clear()

	require.Empty(t, p.Credentials()["aws"], "Credentials after Clear must not return the cached map")
}

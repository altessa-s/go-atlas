// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestidcapture_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/internal/requestidcapture"
)

func TestRecordInDerivedContext(t *testing.T) {
	t.Parallel()
	parent := t.Context()
	requestidcapture.Record(parent, "outside")
	require.Empty(t, requestidcapture.ID(parent))

	capture := requestidcapture.WithCapture(parent)
	child, cancel := context.WithCancel(capture)
	defer cancel()
	requestidcapture.Record(child, "resolved")
	require.Equal(t, "resolved", requestidcapture.ID(capture))
	require.Empty(t, requestidcapture.ID(parent))
}

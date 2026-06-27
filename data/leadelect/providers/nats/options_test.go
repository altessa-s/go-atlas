// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

func TestOptions_StorageDefaultsToMemory(t *testing.T) {
	t.Parallel()
	o := newOptions()
	require.Equal(t, jetstream.MemoryStorage, o.storage,
		"the election bucket must default to ephemeral memory storage")
}

func TestOptions_WithStorageFileOverridesDefault(t *testing.T) {
	t.Parallel()
	o := newOptions(WithStorage(jetstream.FileStorage))
	require.Equal(t, jetstream.FileStorage, o.storage,
		"WithStorage must switch the bucket to durable file storage")
}

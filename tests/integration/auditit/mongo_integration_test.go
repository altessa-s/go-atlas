// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auditit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/audit"
	"github.com/altessa-s/go-atlas/audit/storages/storagetest"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	auditmongo "github.com/altessa-s/go-atlas/audit/storages/mongo"
)

func newMongoStorage(t *testing.T) audit.Storage {
	t.Helper()
	s, err := auditmongo.New(t.Context(), livebackend.MongoDatabase(t, "audit"))
	require.NoError(t, err)
	return s
}

func TestMongoConformance(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, newMongoStorage, mongo.IsDuplicateKeyError)
}

func TestMongoStoreBatchPartialFailure(t *testing.T) {
	t.Parallel()
	s := newMongoStorage(t)
	at := time.Now().UTC()
	first := event("first", at)
	err := storagetest.BatchFailure(t, s, []*audit.Event{first, first, event("after", at)}, []string{first.ID})
	require.True(t, mongo.IsDuplicateKeyError(err))
}

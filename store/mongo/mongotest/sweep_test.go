// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongotest_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/store/mongo/mongotest"
)

// TestNewSweepsStaleDatabasesIT: the first New of a prefix drops the databases of that prefix older than the stale age (left by a
// killed run) and keeps a recent one (a concurrent run), a name without a creation time and a database of another prefix.
func TestNewSweepsStaleDatabasesIT(t *testing.T) {
	t.Parallel()

	// A prefix of its own, so the once-per-process sweep is this test's and no other test's database is at stake.
	tag := make([]byte, 4)
	_, _ = rand.Read(tag)
	prefix := "mongotest_sweep_" + hex.EncodeToString(tag) + "_"

	seed := mongotest.New(t, mongotest.WithStaleAge(0))
	client := seed.Client()
	now := time.Now()
	named := func(prefix string, at time.Time) string {
		return fmt.Sprintf("%s%08x_%s", prefix, at.Unix(), "0badc0de")
	}
	stale := named(prefix, now.Add(-3*time.Hour))
	recent := named(prefix, now.Add(-10*time.Minute))
	legacy := prefix + "0badc0de"
	other := named("mongotest_other_"+hex.EncodeToString(tag)+"_", now.Add(-3*time.Hour))
	for _, name := range []string{stale, recent, legacy, other} {
		_, err := client.Database(name).Collection("c").InsertOne(t.Context(), bson.D{{Key: "x", Value: 1}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Database(name).Drop(context.WithoutCancel(t.Context())) })
	}

	m := mongotest.New(t, mongotest.WithDatabasePrefix(prefix), mongotest.WithStaleAge(time.Hour))
	require.Regexp(t, regexp.MustCompile(`^`+regexp.QuoteMeta(prefix)+`[0-9a-f]{8}_[0-9a-f]{8}$`), m.DatabaseName())
	created, err := strconv.ParseInt(m.DatabaseName()[len(prefix):len(prefix)+8], 16, 64)
	require.NoError(t, err)
	require.WithinDuration(t, now, time.Unix(created, 0), time.Minute)

	names, err := client.ListDatabaseNames(t.Context(), bson.D{{Key: "name", Value: bson.D{{Key: "$in", Value: bson.A{
		stale, recent, legacy, other,
	}}}}})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{recent, legacy, other}, names)
}

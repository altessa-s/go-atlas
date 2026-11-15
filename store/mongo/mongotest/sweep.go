// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongotest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// sweepTimeout bounds the listing and dropping of stale databases.
const sweepTimeout = 30 * time.Second

// sweeps holds one sync.OnceFunc per URI and prefix: the stale databases are swept by the first New of the process only.
var sweeps sync.Map

// databaseName returns a throwaway database name: the prefix, the creation time as 8 hex digits of Unix seconds, an underscore and
// 8 random hex digits.
func databaseName(prefix string, now time.Time) string {
	suffix := make([]byte, databaseSuffixBytes)
	_, _ = rand.Read(suffix)
	return fmt.Sprintf("%s%08x_%s", prefix, now.Unix(), hex.EncodeToString(suffix))
}

// sweepStale drops, once per process for uri and prefix, every database named like a throwaway database of prefix and created
// more than staleAge ago: the leftovers of runs killed before their cleanup. A name without a creation time is never dropped. The
// sweep is best effort; a failure is logged, not reported.
func sweepStale(tb testing.TB, client *mongo.Client, uri, prefix string, staleAge time.Duration) {
	if staleAge <= 0 {
		return
	}
	sweep, _ := sweeps.LoadOrStore(uri+"\x00"+prefix, sync.OnceFunc(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(tb.Context()), sweepTimeout)
		defer cancel()
		dropped, err := dropStale(ctx, client, prefix, time.Now().Add(-staleAge))
		if err != nil {
			tb.Logf("mongotest: sweep stale %s databases: %v", prefix, err)
		}
		if dropped > 0 {
			tb.Logf("mongotest: dropped %d stale %s databases", dropped, prefix)
		}
	}))
	if run, ok := sweep.(func()); ok {
		run()
	}
}

// dropStale drops the throwaway databases of prefix created before cutoff and returns how many it dropped.
func dropStale(ctx context.Context, client *mongo.Client, prefix string, cutoff time.Time) (int, error) {
	quoted := regexp.QuoteMeta(prefix)
	names, err := client.ListDatabaseNames(ctx, bson.D{{Key: "name", Value: bson.D{{Key: "$regex", Value: "^" + quoted}}}})
	if err != nil {
		return 0, err
	}
	pattern := regexp.MustCompile(`^` + quoted + `([0-9a-f]{8})_[0-9a-f]{8}$`)
	dropped := 0
	for _, name := range names {
		m := pattern.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		secs, err := strconv.ParseInt(m[1], 16, 64)
		if err != nil || !time.Unix(secs, 0).Before(cutoff) {
			continue
		}
		if err := client.Database(name).Drop(ctx); err != nil {
			return dropped, err
		}
		dropped++
	}
	return dropped, nil
}

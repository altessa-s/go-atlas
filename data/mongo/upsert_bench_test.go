// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/shopspring/decimal"

	datamongo "github.com/altessa-s/go-atlas/data/mongo"
)

func BenchmarkSetOnInsertFrom(b *testing.B) {
	doc := snapshotDoc{
		ID: "cbr:USD:RUB:2026-10-18:1", Source: 1, Base: "USD", Value: decimal.RequireFromString("81.2345"),
		Effective: civil.Date{Year: 2026, Month: time.October, Day: 18}, FetchedAt: time.Now(), Version: 1,
	}
	for b.Loop() {
		if _, err := datamongo.SetOnInsertFrom(doc, "source", "base", "effective_date", "version"); err != nil {
			b.Fatal(err)
		}
	}
}

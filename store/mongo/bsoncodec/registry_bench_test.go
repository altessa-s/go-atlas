// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec_test

import (
	"testing"

	"cloud.google.com/go/civil"
)

func BenchmarkCivilDateRoundTrip(b *testing.B) {
	in := struct {
		Day civil.Date `bson:"day"`
	}{Day: civil.Date{Year: 2026, Month: 10, Day: 9}}
	b.ReportAllocs()
	for b.Loop() {
		raw, err := marshal(in)
		if err != nil {
			b.Fatal(err)
		}
		var out struct {
			Day civil.Date `bson:"day"`
		}
		if err := unmarshal(raw, &out); err != nil {
			b.Fatal(err)
		}
	}
}

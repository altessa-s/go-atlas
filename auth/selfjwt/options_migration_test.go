// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"reflect"
	"testing"

	jwt "github.com/altessa-s/go-atlas/auth/jwt"
)

func TestOptionsMigrationBaseline(t *testing.T) {
	got := defaultOptions()
	before := []jwt.Algorithm{jwt.Algorithm(" HS256 "), jwt.Algorithm("HS256"), jwt.Algorithm("HS256")}
	WithAllowedAlgorithms()(got)
	if !reflect.DeepEqual(got.allowedAlgorithms, defaultAllowedAlgorithms) {
		t.Fatal("empty call cleared default algorithms")
	}
	WithAllowedAlgorithms(before...)(got)
	before[0] = jwt.AlgRS256
	if !reflect.DeepEqual(got.allowedAlgorithms, []jwt.Algorithm{" HS256 ", "HS256", "HS256"}) {
		t.Fatalf("input was not copied or was normalized: %q", got.allowedAlgorithms)
	}
}

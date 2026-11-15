// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt

import (
	"reflect"
	"testing"
)

func TestOptionsMigrationBaseline(t *testing.T) {
	t.Run("allowed algorithms", func(t *testing.T) {
		before := []Algorithm{Algorithm(" HS256 "), Algorithm("HS256"), Algorithm("HS256")}
		got := defaultOptions()
		WithAllowedAlgorithms()(got)
		if !reflect.DeepEqual(got.allowedAlgorithms, DefaultAllowedAlgorithms()) {
			t.Fatal("empty call cleared default algorithms")
		}
		WithAllowedAlgorithms(before...)(got)
		before[0] = AlgRS256
		if !reflect.DeepEqual(got.allowedAlgorithms, []Algorithm{" HS256 ", "HS256", "HS256"}) {
			t.Fatalf("input was not copied or was normalized: %q", got.allowedAlgorithms)
		}
	})
	t.Run("audiences", func(t *testing.T) {
		got := defaultOptions()
		got.audiences = []string{"existing"}
		WithAudiences()(got)
		if !reflect.DeepEqual(got.audiences, []string{"existing"}) {
			t.Fatalf("empty call changed audiences: %v", got.audiences)
		}
		input := []string{" api ", "api"}
		WithAudiences(input...)(got)
		input[0] = "changed"
		if !reflect.DeepEqual(got.audiences, []string{" api ", "api"}) {
			t.Fatalf("audiences not copied or normalized: %q", got.audiences)
		}
	})
	t.Run("required claims", func(t *testing.T) {
		got := defaultOptions()
		got.requiredClaims = []string{"existing"}
		WithRequiredClaims()(got)
		if !reflect.DeepEqual(got.requiredClaims, []string{"existing"}) {
			t.Fatalf("empty call changed claims: %v", got.requiredClaims)
		}
		input := []string{" custom ", "custom"}
		WithRequiredClaims(input...)(got)
		input[0] = "changed"
		if !reflect.DeepEqual(got.requiredClaims, []string{" custom ", "custom"}) {
			t.Fatalf("claims not copied or normalized: %q", got.requiredClaims)
		}
	})
}

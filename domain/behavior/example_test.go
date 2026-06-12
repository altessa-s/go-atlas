// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior_test

import (
	"fmt"

	"github.com/altessa-s/go-atlas/domain/behavior"
)

func ExampleStripCreate() {
	type Bucket struct {
		ID       string `behavior:"identifier"`
		TenantID string `behavior:"immutable"`
		Name     string
	}

	// A client supplied a server-owned ID; StripCreate clears it but keeps the
	// immutable TenantID (immutable is only stripped on update).
	b := Bucket{ID: "client-supplied", TenantID: "t1", Name: "photos"}
	if err := behavior.StripCreate(&b); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("%+v\n", b)
	// Output: {ID: TenantID:t1 Name:photos}
}

func ExampleStripUpdate() {
	type Bucket struct {
		TenantID string `behavior:"immutable"`
		Name     string
	}

	// On update the immutable field is cleared so it cannot be changed.
	b := Bucket{TenantID: "t1", Name: "photos"}
	if err := behavior.StripUpdate(&b); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("%+v\n", b)
	// Output: {TenantID: Name:photos}
}

func ExampleStrip_strict() {
	type Bucket struct {
		ID string `behavior:"identifier"`
	}

	// Strict mode reports populated fields that would be stripped instead of
	// mutating the struct.
	b := Bucket{ID: "x"}
	err := behavior.StripCreate(&b, behavior.WithStrict())

	fmt.Println(err)
	fmt.Printf("unchanged: %q\n", b.ID)
	// Output:
	// behavior: 1 violation(s): ID (identifier)
	// unchanged: "x"
}

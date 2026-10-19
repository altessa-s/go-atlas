// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior_test

import (
	"context"
	"fmt"
	"slices"

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
		ID       string `behavior:"identifier"`
		TenantID string `behavior:"immutable"`
		Name     string
	}

	// On update the immutable field is cleared so it cannot be changed; the
	// identifier is kept because it addresses the resource.
	b := Bucket{ID: "b1", TenantID: "t1", Name: "photos"}
	if err := behavior.StripUpdate(&b); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("%+v\n", b)
	// Output: {ID:b1 TenantID: Name:photos}
}

func ExampleStripUpdate_formerKinds() {
	type Bucket struct {
		ID       string `behavior:"identifier"`
		TenantID string `behavior:"immutable"`
		Name     string
	}

	// The strip set StripUpdate used before it kept the identifier: WithKinds
	// replaces the default set.
	b := Bucket{ID: "b1", TenantID: "t1", Name: "photos"}
	if err := behavior.StripUpdate(&b, behavior.WithKinds(behavior.OutputOnly, behavior.Identifier, behavior.Immutable)); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("%+v\n", b)
	// Output: {ID: TenantID: Name:photos}
}

// columns folds the top-level fields of an update into column values. The
// engine no longer strips the identifier under DefaultUpdateKinds, so the
// translator leaves it out itself: it selects the row to update.
type columns struct{}

func (columns) Translate(_ context.Context, root behavior.Object) (map[string]any, error) {
	out := make(map[string]any, len(root.Fields))
	for _, f := range root.Fields {
		if f.Strip || slices.Contains(f.Kinds, behavior.Identifier) {
			continue
		}
		out[f.Name] = f.Value.Interface()
	}
	return out, nil
}

func ExampleNew_identifierExcluded() {
	type Bucket struct {
		ID       string `behavior:"identifier"`
		TenantID string `behavior:"immutable"`
		Name     string
	}

	eng := behavior.New[map[string]any](columns{}, behavior.WithKinds(behavior.DefaultUpdateKinds...))
	cols, err := eng.Translate(context.Background(), &Bucket{ID: "b1", TenantID: "t1", Name: "photos"})
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(cols)
	// Output: map[Name:photos]
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

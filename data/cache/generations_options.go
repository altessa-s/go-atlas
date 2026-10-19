// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=generationsOptions --output=generations_options_gen.go --option-type=GenerationsOption

import (
	"context"

	"github.com/altessa-s/go-atlas/data/cache/generation/storages/memory"
)

// generationsOptions configures [NewGenerations].
type generationsOptions struct {
	// store holds the generations (default: a new in-process store). Use a
	// shared store (data/cache/generation/storages/redis) to invalidate the
	// caches of every replica.
	store GenerationStore `opt:"GenerationStore" optgen:"default=defaultGenerationStore(),notnil"`
	// bypass reports whether a read must skip the cache, for example because
	// ctx carries a database transaction whose uncommitted writes the cache
	// must neither serve nor store. Nil never bypasses (beyond
	// [WithFreshReads]).
	bypass func(ctx context.Context) bool `opt:"Bypass"`
}

// defaultGenerationStore returns the in-process store a [Generations] uses
// unless [WithGenerationStore] is given.
func defaultGenerationStore() GenerationStore { return memory.New() }

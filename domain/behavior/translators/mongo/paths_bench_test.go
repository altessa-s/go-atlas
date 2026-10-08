// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/domain/behavior"

	mongo "github.com/altessa-s/go-atlas/domain/behavior/translators/mongo"
)

func BenchmarkFieldPathsTranslator(b *testing.B) {
	eng := behavior.New[mongo.FieldPaths](mongo.NewFieldPathsTranslator(),
		behavior.WithKinds(behavior.DefaultResponseKinds...), behavior.WithSchemaWalk())
	ctx := b.Context()
	for b.Loop() {
		_, _ = eng.Translate(ctx, user{})
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlstore

import (
	"context"
	"time"

	corecontext "github.com/altessa-s/go-atlas-core/context"
)

// EnsureSchemaTimeout bounds optional startup schema creation so a lock wait
// cannot stall service startup indefinitely.
const EnsureSchemaTimeout = 30 * time.Second

// SchemaInitializer is implemented by SQL reliability stores that provision
// their own schema idempotently at startup.
type SchemaInitializer interface {
	EnsureSchema(ctx context.Context) error
}

// Build constructs a SQL store with create and, when ensureSchema is true,
// runs store.EnsureSchema under a context bounded by [EnsureSchemaTimeout].
// A failure in EnsureSchema is passed through wrapSchemaErr when non-nil.
func Build[S SchemaInitializer](
	ctx context.Context,
	ensureSchema bool,
	create func() (S, error),
	wrapSchemaErr func(error) error,
) (S, error) {
	var zero S
	store, err := create()
	if err != nil || !ensureSchema {
		return store, err
	}
	schemaCtx, cancel := corecontext.WithMaxTimeout(ctx, EnsureSchemaTimeout)
	defer cancel()
	if err := store.EnsureSchema(schemaCtx); err != nil {
		if wrapSchemaErr != nil {
			err = wrapSchemaErr(err)
		}
		return zero, err
	}
	return store, nil
}

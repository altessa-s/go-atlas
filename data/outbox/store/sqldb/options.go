// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"context"
	"time"
)

// DefaultTableName is the default table name for outbox events; it matches the
// MongoDB store's default collection name.
const DefaultTableName = "events_outbox"

// DefaultSchemaCreateTimeout bounds the table and index creation done by [New].
const DefaultSchemaCreateTimeout = 10 * time.Second

type options struct {
	tableName     string          `optval:"nonempty" optgen:"default=DefaultTableName"`
	ctx           context.Context `opt:"Context" optgen:"notnil"`
	schemaTimeout time.Duration   `opt:"SchemaCreateTimeout" optval:"positive" optgen:"default=DefaultSchemaCreateTimeout"`
}

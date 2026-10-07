// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

// DefaultTableName is the default table name for saga instances; it matches the
// MongoDB store's default collection name.
const DefaultTableName = "saga_instances"

type options struct {
	tableName string `optval:"nonempty" optgen:"default=DefaultTableName"`
}

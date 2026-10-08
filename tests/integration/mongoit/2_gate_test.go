// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"context"
	"errors"
	"strings"

	migrate "github.com/xakep666/mongo-migrate"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// failingPrefix marks the databases on which migration 2 fails.
const failingPrefix = "mongoit_fail_"

// errGate is the failure migration 2 reports.
var errGate = errors.New("migration 2 failed")

func init() {
	migrate.MustRegister(func(_ context.Context, db *mongo.Database) error {
		if strings.HasPrefix(db.Name(), failingPrefix) {
			return errGate
		}
		return nil
	}, func(context.Context, *mongo.Database) error { return nil })
}

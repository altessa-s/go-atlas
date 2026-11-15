// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"

	"github.com/altessa-s/go-atlas/store/mongo/migrations"
)

func (m *Mongo) migrate(ctx context.Context) error {
	opts := []migrations.Option{migrations.WithLogger(m.config.Logger)}
	if m.config.RollbackMigrationsOnFailure {
		opts = append(opts, migrations.WithRollbackOnFailure())
	}
	return migrations.Run(ctx, m.client.Database(m.DatabaseName()), opts...)
}

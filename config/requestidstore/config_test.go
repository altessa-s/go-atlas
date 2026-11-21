// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestidstoreconfig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	cfg "github.com/altessa-s/go-atlas/config/requestidstore"
)

func TestSelectedBlock(t *testing.T) {
	t.Parallel()
	for _, typ := range []cfg.StorageType{cfg.StorageTypeNATS, cfg.StorageTypeRedis, cfg.StorageTypeMongo, cfg.StorageTypeSQL} {
		t.Run(string(typ), func(t *testing.T) {
			t.Parallel()
			c := cfg.Config{Type: typ}
			require.Error(t, c.Validate())
			c.Normalize()
			switch typ {
			case cfg.StorageTypeNATS:
				c.NATS.Bucket = "request_ids"
				c.NATS.Replicas = 1
			case cfg.StorageTypeSQL:
				c.SQL.Table = "request_ids"
				c.SQL.Dialect = "postgres"
				c.SQL.PurgeBatchSize = 1000
			case cfg.StorageTypeMongo:
				c.Mongo.Collection = "request_ids"
			case cfg.StorageTypeRedis:
				require.NotNil(t, c.Redis)
			}
			require.NoError(t, c.Validate())
		})
	}
}

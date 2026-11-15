// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"

	"github.com/altessa-s/go-atlas/coord/limiters/storages"

	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
)

// StorageFunc creates the limiter storage of one storage type from the
// storage configuration and the NATS contention settings of the limiter
// (nil when unset). ctx bounds storage provisioning, such as creating a NATS
// KeyValue bucket.
type StorageFunc func(ctx context.Context, cfg *storageconfig.BackendConfig, contention *limiterconfig.NATSContention) (storages.Storage, error)

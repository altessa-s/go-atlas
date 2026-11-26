// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"

	"github.com/altessa-s/go-atlas/coord/limiters/storages"

	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
)

// StorageFunc creates the limiter storage of one storage type from the
// limiter storage configuration. ctx bounds storage provisioning.
type StorageFunc func(ctx context.Context, cfg *limiterconfig.StorageConfig) (storages.Storage, error)

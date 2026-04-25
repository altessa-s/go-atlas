// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storages

import "errors"

// ErrEmptyKey is returned by [Storage] implementations when an empty
// idempotency key is supplied. An empty key would silently disable dedupe
// — every "lock" would succeed without writing anything — so callers must
// validate keys before reaching the storage layer.
var ErrEmptyKey = errors.New("idempotency: empty key")

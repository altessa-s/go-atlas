// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"github.com/altessa-s/go-atlas/core/runtime/singleflight"
	"github.com/altessa-s/go-atlas/data/cache/storages"
)

// ErrMissing is returned when no cache is found.
var ErrMissing = storages.ErrMissing

// ErrOverloaded indicates that the limit of distinct active fallbacks was reached.
var ErrOverloaded = singleflight.ErrOverloaded

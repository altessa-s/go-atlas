// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"time"
)

// DefaultHandlerTimeout is the default timeout for callback execution (3 seconds).
const DefaultHandlerTimeout = 3 * time.Second

// options contains Leader configuration.
type options struct {
	handlerTimeout time.Duration `optgen:"default=DefaultHandlerTimeout"`
}

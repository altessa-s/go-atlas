// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retryqueue

import (
	"fmt"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

var (
	// ErrItemNotFound reports that the key an operation must find an item
	// under has none: [Mover.Move] from an empty key. The [Storage]
	// transitions never return it (a transition on a missing item is a
	// no-op). It wraps [coreerrs.ErrNotFound].
	ErrItemNotFound = fmt.Errorf("retryqueue: item %w", coreerrs.ErrNotFound)

	// ErrItemExists reports that the key an operation must write an item
	// under already holds one: [Mover.Move] onto a taken key. It wraps
	// [coreerrs.ErrAlreadyExists].
	ErrItemExists = fmt.Errorf("retryqueue: item %w", coreerrs.ErrAlreadyExists)
)

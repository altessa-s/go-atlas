// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package query

import (
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/store/mongo/bsoncodec"
)

// DefaultTimeout caps read operations.
const DefaultTimeout = 5 * time.Second

// CloseTimeout bounds cursor cleanup independently of caller cancellation.
const CloseTimeout = 5 * time.Second

// Registry is the read-only default for operations without a client registry.
var Registry = sync.OnceValue(bsoncodec.NewRegistry)

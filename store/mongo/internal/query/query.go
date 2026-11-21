// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package query

import (
	"context"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/altessa-s/go-mongox/bsonfilter"

	"github.com/altessa-s/go-atlas/store/mongo/bsoncodec"
)

// DefaultTimeout caps read operations.
const DefaultTimeout = 5 * time.Second

// CloseTimeout bounds cursor cleanup independently of caller cancellation.
const CloseTimeout = 5 * time.Second

// Registry is the shared default document encoding registry. Treat it as read-only.
var Registry = sync.OnceValue(bsoncodec.NewRegistry)

// ErrDangerousOperator identifies a filter containing a server-side JavaScript operator.
var ErrDangerousOperator = bsonfilter.ErrDangerousOperator

// ValidateFilter returns the validated bytes to send to MongoDB.
func ValidateFilter(ctx context.Context, filter any, registry *bson.Registry, opts *options.BSONOptions) (bson.Raw, error) {
	return bsonfilter.Prepare(ctx, filter, registry, opts)
}

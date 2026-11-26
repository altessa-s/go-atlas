// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package updatevalidation

import (
	"context"

	"github.com/altessa-s/go-aip/fieldmask/update"

	"google.golang.org/protobuf/proto"
)

// SpecLookup selects the handler-owned spec for a unary RPC.
type SpecLookup func(context.Context) *update.Spec

// Normalization contains the result fields used by the mask interceptors.
type Normalization struct {
	Located  bool
	Resource proto.Message
	Paths    int
	Plan     *update.DeferredPlan
}

// The updatemask package initializes these operations. Keeping them here lets
// the factory and interceptors use them without exposing the internal options
// and handoff methods on updatemask's public API or creating an import cycle.
var (
	NewSpecLookupOption func(SpecLookup) any
	HasRegisteredSpec   func(context.Context, any) bool
	Normalize           func(context.Context, any, proto.Message, bool) (context.Context, Normalization, error)
)

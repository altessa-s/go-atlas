// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"context"

	corecontext "github.com/altessa-s/go-atlas/core/context"
)

type requestIDContextKey struct{}

// NewContext returns a child context carrying id as the request identifier.
// A nil ctx is treated as [context.Background].
func NewContext(ctx context.Context, id string) context.Context {
	return context.WithValue(corecontext.OrBackground(ctx), requestIDContextKey{}, id)
}

// FromContext extracts the request ID stored by [NewContext].
// Returns an empty string when the context is nil or does not carry a
// request ID.
func FromContext(ctx context.Context) string {
	id, ok := corecontext.OrBackground(ctx).Value(requestIDContextKey{}).(string)
	if !ok {
		return ""
	}
	return id
}

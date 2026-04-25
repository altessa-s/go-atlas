// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"context"

	corecontext "github.com/altessa-s/go-atlas/core/context"
)

type auditorContextKey struct{}

// NewContext returns a new context with the given Auditor stored.
func NewContext(ctx context.Context, a *Auditor) context.Context {
	return context.WithValue(corecontext.OrBackground(ctx), auditorContextKey{}, a)
}

// FromContext returns the Auditor from the context, or nil if not present.
func FromContext(ctx context.Context) *Auditor {
	a, _ := corecontext.OrBackground(ctx).Value(auditorContextKey{}).(*Auditor) //nolint:errcheck // type assertion may fail; nil is acceptable
	return a
}

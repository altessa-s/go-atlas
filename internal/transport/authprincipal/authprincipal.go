// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authprincipal

import (
	"context"

	"github.com/altessa-s/go-atlas/auth/principal"
)

// source is implemented by authentication results that carry their own
// mapping onto the canonical principal, such as *oidc.Claims.
type source interface {
	Principal() principal.Principal
}

// Bridge returns ctx carrying data through [principal.NewContextFrom] when data
// is a [principal.Principal] or a non-nil *principal.Principal; a Principal
// value is stored without boxing it again. When data instead maps itself onto
// a principal through a Principal() principal.Principal method, as
// *oidc.Claims does, the non-zero result is stored. Any other value, including
// an application-defined identity type, leaves ctx unchanged.
func Bridge(ctx context.Context, data any) context.Context {
	if out, ok := principal.NewContextFrom(ctx, data); ok {
		return out
	}
	if src, ok := data.(source); ok {
		if p := src.Principal(); !p.IsZero() {
			return principal.NewContext(ctx, p)
		}
	}
	return ctx
}

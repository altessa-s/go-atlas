// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package authprincipal bridges an authenticator's result into
// [github.com/altessa-s/go-atlas/auth/principal.NewContextFrom]. The HTTP auth
// middleware and the gRPC auth interceptor share it so that, whenever an
// authenticator returns the canonical principal or a result that maps itself
// onto one (such as *oidc.Claims), a handler served over both transports
// reads the caller with principal.FromContext.
//
//	ctx = authprincipal.Bridge(ctx, data)
package authprincipal

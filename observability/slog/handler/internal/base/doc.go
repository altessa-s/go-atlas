// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package base provides a base implementation for slog handler middlewares.
//
// This package extracts common patterns used in slog middleware handlers that wrap
// an inner handler, delegating to it while adding their own processing logic.
//
// # Base Handler
//
// The [Base] type provides common functionality for middleware handlers:
//   - Inner handler storage and delegation
//   - Group path tracking for nested attribute access
//   - Enabled() delegation to inner handler
//
// Example usage:
//
//	type MyHandler struct {
//	    base.Base
//	    customField string
//	}
//
//	func NewMyHandler(inner slog.Handler) *MyHandler {
//	    return &MyHandler{
//	        Base:        base.NewBase(inner),
//	        customField: "value",
//	    }
//	}
//
//	func (h *MyHandler) Handle(ctx context.Context, r slog.Record) error {
//	    // Custom processing...
//	    return h.Inner().Handle(ctx, r)
//	}
//
//	func (h *MyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
//	    return &MyHandler{
//	        Base:        h.WithAttrsBase(attrs),
//	        customField: h.customField,
//	    }
//	}
//
//	func (h *MyHandler) WithGroup(name string) slog.Handler {
//	    if name == "" {
//	        return h
//	    }
//	    return &MyHandler{
//	        Base:        h.WithGroupBase(name),
//	        customField: h.customField,
//	    }
//	}
//
// # Group Path Tracking
//
// The Base type tracks the current group path, which is useful for handlers that
// need to know the full attribute path (e.g., for masking "user.password"):
//
//	groups := h.Groups() // Returns []string{"user"} inside WithGroup("user")
//	path := h.GroupPath("password") // Returns "user.password"
package base

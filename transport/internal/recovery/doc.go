// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package recovery captures stack traces during panic recovery and wraps
// the recovered value into a structured [PanicError].
//
// [StackTrace] walks the call stack using [runtime.Callers] and returns
// a slice of [Frame] values ([Frames]) that can be serialized to JSON
// via [Frames.MarshalJSON] or rendered as a human-readable string via
// [Frames.String].
//
// Typical usage inside a deferred recover:
//
//	defer func() {
//	    if r := recover(); r != nil {
//	        err := recovery.NewPanicError(r, 0)
//	        logger.Error("panic", "error", err, "stack", err.Frames)
//	    }
//	}()
package recovery

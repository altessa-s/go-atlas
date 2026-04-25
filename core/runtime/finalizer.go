// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package runtime

import (
	"runtime"
)

// Cleanup represents a registered cleanup task that can be canceled before it runs.
// Calling [Cleanup.Stop] prevents the cleanup function from being called, even if
// the associated object becomes unreachable. It is safe to call Stop multiple times.
type Cleanup interface {
	Stop()
}

// AddCleanup attaches a cleanup function to obj that will be called some time
// after obj becomes unreachable. The cleanup function receives arg as its parameter
// and runs in a separate goroutine. The returned [Cleanup] can be used to cancel
// the cleanup before it fires. This is a thin wrapper around runtime.AddCleanup
// (introduced in Go 1.24).
func AddCleanup[T, S any](obj *T, cleanup func(S), arg S) Cleanup {
	return runtime.AddCleanup(obj, cleanup, arg)
}

// ClearFinalizer removes any finalizer previously set on obj via runtime.SetFinalizer.
// It is safe to call even if no finalizer was set. Internally it calls
// runtime.SetFinalizer(obj, nil).
func ClearFinalizer[T any](obj *T) {
	runtime.SetFinalizer(obj, nil)
}

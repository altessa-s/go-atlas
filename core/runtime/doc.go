// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package runtime provides process-wide and scoped shutdown hook registries.
//
// # Shutdown scopes
//
// [OnShutdown] and [RunShutdownHooks] register into one implicit group whose
// lifetime is the process. A component whose lifetime is shorter — one owned by
// a factory, a test, or a subsystem that is torn down and rebuilt — uses its own
// [HookGroup] instead, so it can be stopped without stopping the program:
//
//	var hooks runtime.HookGroup // zero value is ready to use
//	hooks.OnShutdown(engine.Stop)
//	...
//	err := hooks.Shutdown(ctx)
package runtime

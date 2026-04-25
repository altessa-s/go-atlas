// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugin

import "sync/atomic"

// warnSinkValue holds the warning sink function atomically.
// Using atomic.Value is more efficient than RWMutex for read-heavy workloads:
// the sink is set once at startup, but warnf is called many times.
var warnSinkValue atomic.Value

// warnFunc is the type stored in warnSinkValue.
type warnFunc func(format string, args ...any)

// SetWarningSink installs a process-wide callback for optgen warnings. Pass nil
// to disable warnings (the default). The sink is stored via [atomic.Value] so
// it can be read without locking on every warnf call.
//
// Typically called once at CLI startup, before any generation runs.
func SetWarningSink(sink func(format string, args ...any)) {
	if sink == nil {
		warnSinkValue.Store(warnFunc(nil))
	} else {
		warnSinkValue.Store(warnFunc(sink))
	}
}

func warnf(format string, args ...any) {
	v := warnSinkValue.Load()
	if v == nil {
		return
	}
	if sink := v.(warnFunc); sink != nil { //nolint:errcheck
		sink(format, args...)
	}
}

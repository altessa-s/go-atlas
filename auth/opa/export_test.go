// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

// SchedulerCallbackAttached reports whether a retained scheduler handle still
// holds the manager's reload guard. Closed-manager ticks alone cannot tell.
func (m *Manager) SchedulerCallbackAttached() bool { return m.scheduled.guard.Load() != nil }

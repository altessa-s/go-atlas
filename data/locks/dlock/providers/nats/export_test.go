// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

// TrackedLocks returns how many locks the provider currently tracks.
func (l *Locker) TrackedLocks() int {
	l.activeLocks.mu.RLock()
	defer l.activeLocks.mu.RUnlock()
	return len(l.activeLocks.locks)
}

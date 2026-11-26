// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package leasing is the lease lifecycle shared by the lock providers that
// keep each lock as a record with an expiry and a fencing token — MongoDB and
// SQL. A provider implements [Store], the five conditional operations on its
// records; [Engine] does the rest: single-attempt acquisition with a
// client-side deadline, rejection of late replies, release of ambiguous
// acquisitions by owner with TTL-bounded tracking, background renewal that stops once a renewal is not
// confirmed within the lease, release on context end, retryable releases, and
// a Close that waits for acquisitions in flight. Failed renewals retry after
// half the remaining lease (at least 1ms), capped by the confirmed deadline.
// Renewal calls are bounded by that deadline; late replies cannot revive a lease.
package leasing

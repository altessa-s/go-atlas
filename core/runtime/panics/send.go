// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package panics

import "context"

// TrySend performs a blocking send of v to ch, recovering the "send on closed
// channel" panic instead of crashing. The send blocks until the value is
// delivered or ctx is done; pass context.Background() for an uncancelable
// blocking send. It returns sent=true when the value was delivered,
// closed=true when the send panicked because ch was closed, and (false,
// false) when ctx was done before the value could be delivered.
//
// Needing TrySend is a channel-ownership smell: in a well-factored design the
// sending side owns the channel and closes it only after all sends have
// stopped, so a send can never observe a closed channel. TrySend legitimizes
// an existing pattern where that ownership is shared or inverted — it does
// not fix the ownership design. Prefer restructuring so the sender controls
// close; reach for TrySend only when that restructuring is impractical.
func TrySend[T any](ctx context.Context, ch chan<- T, v T) (sent, closed bool) {
	defer func() {
		// The only panic a channel send can raise is "send on closed channel".
		if recover() != nil {
			sent, closed = false, true
		}
	}()

	select {
	case ch <- v:
		return true, false
	case <-ctx.Done():
		return false, false
	}
}

// TrySendNonBlocking performs a non-blocking (select-with-default) send of v
// to ch, recovering the "send on closed channel" panic instead of crashing.
// It returns sent=true when the value was delivered, closed=true when the
// send panicked because ch was closed, and (false, false) when the channel
// buffer was full (or ch is nil, which always takes the default case).
//
// The same channel-ownership caveat as [TrySend] applies: this helper
// legitimizes sends racing with close, it does not fix the ownership design.
func TrySendNonBlocking[T any](ch chan<- T, v T) (sent, closed bool) {
	defer func() {
		// The only panic a channel send can raise is "send on closed channel".
		if recover() != nil {
			sent, closed = false, true
		}
	}()

	select {
	case ch <- v:
		return true, false
	default:
		return false, false
	}
}

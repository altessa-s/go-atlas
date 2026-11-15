// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import "context"

// Handler is a function that dispatches an event to an external system.
// It is called by the Outbox during the dispatch cycle for each event.
// Returning an error triggers the retry logic.
//
// Example:
//
//	handler := func(ctx context.Context, event outbox.Event) error {
//	    return broker.Publish(ctx, event.Key, event.Payload)
//	}
type Handler func(ctx context.Context, event Event) error

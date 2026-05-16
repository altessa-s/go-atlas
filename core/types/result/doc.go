// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package result provides a generic Result[T] type that holds either
// a value of type T or an error. It is meant for places where Go's
// idiomatic (T, error) tuple is awkward to express — channel
// elements, slice values, map values — not as a general replacement
// for (T, error) returns.
//
// All exported constructors and methods are pure and safe for
// concurrent use. The zero value of Result[T] is a valid Ok of the
// zero value of T.
//
// # Usage
//
//	// chan Result[T]: fan-out where each item independently succeeds
//	// or fails, and the consumer wants both successes and failures.
//	results := make(chan result.Result[Page])
//	go func() {
//	    defer close(results)
//	    for _, url := range urls {
//	        results <- result.Of(fetch(url))
//	    }
//	}()
//	for r := range results {
//	    page, err := r.Get()
//	    if err != nil {
//	        log.Warn("fetch", "err", err)
//	        continue
//	    }
//	    process(page)
//	}
//
// # When NOT to use
//
// For ordinary synchronous calls keep returning (T, error) and use
// `if err != nil`. For panic-on-error wrap a Result via the existing
// helper instead of adding an Unwrap method:
//
//	v := panics.MustResult(r.Get())
//
// For asynchronous fan-out with aggregated errors prefer
// core/runtime/concurrency.ProcessCollect over a hand-rolled
// chan Result[T].
package result

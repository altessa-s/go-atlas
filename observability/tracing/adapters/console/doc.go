// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package console provides a console [adapters.Adapter] for tracing.
// It outputs [adapters.SpanData] to stdout/stderr in human-readable or JSON format.
// Primarily intended for development and debugging.
//
// # Example
//
//	adapter := console.New(
//	    console.WithWriter(os.Stdout),
//	    console.WithPrettyPrint(false), // JSON lines instead of the human-readable form
//	    console.WithTimestamps(true),
//	)
//	provider := tracing.New(tracing.WithAdapter(adapter))
//
// Pretty printing and timestamps both default to true ([DefaultPrettyPrint],
// [DefaultTimestamps]); pass false to [WithPrettyPrint] or [WithTimestamps]
// to turn them off.
package console

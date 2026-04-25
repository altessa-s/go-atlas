// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package json provides a JSON [codec.Codec] and [codec.StreamingEncoder]
// backed by encoding/json.
//
// The [Codec] is safe for concurrent use and supports two MIME types:
// [MimeType] ("application/json") and [AlternateMimeType] ("text/json").
// Both are registered in the global [codec.DefaultRegistry] at init time.
//
// HTML escaping is enabled by default. Use [WithEscapeHTML](false) to disable
// it when embedding JSON in non-HTML contexts.
//
// # Example
//
//	codec := json.New(json.WithIndent(true), json.WithEscapeHTML(false))
//	data, _ := codec.Encode(response)
package json

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package xml provides an XML [codec.Codec] backed by encoding/xml.
//
// The [Codec] is safe for concurrent use and supports two MIME types:
// [MimeType] ("application/xml") and [AlternateMimeType] ("text/xml").
// Both are registered in the global [codec.DefaultRegistry] at init time.
//
// Use [WithHeader](true) to prepend the standard XML declaration
// (<?xml version="1.0" encoding="UTF-8"?>) to encoded output.
//
// # Example
//
//	codec := xml.New(xml.WithIndent(true), xml.WithHeader(true))
//	data, _ := codec.Encode(response)
package xml

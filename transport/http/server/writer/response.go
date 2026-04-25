// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import "encoding/xml"

// Response is the default response envelope produced by [Default.Build].
// It wraps either successful data or a structured [Error].
type Response struct {
	XMLName xml.Name `json:"-" xml:"Response"`
	Data    any      `json:"data,omitempty" xml:"Data,omitempty"`
	Error   *Error   `json:"error,omitempty" xml:"Error,omitempty"`
}

// Error represents a structured error response.
// It contains a machine-readable code and human-readable message.
type Error struct {
	XMLName xml.Name `json:"-" xml:"Error"`
	Code    string   `json:"code,omitempty" xml:"Code,omitempty"`
	Message string   `json:"message,omitempty" xml:"Message,omitempty"`
}

// ErrorConverter converts a Go error into a structured [Error] with an HTTP status code.
// Return status code 0 to use the default (500 Internal Server Error).
// Set it on the [Default] builder via [WithDefaultBuilderErrorConverter].
type ErrorConverter func(error) (Error, int)

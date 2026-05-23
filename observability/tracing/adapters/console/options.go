// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package console

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import "io"

// Default values for console adapter options.
const (
	// DefaultPrettyPrint enables human-readable output by default.
	DefaultPrettyPrint = true

	// DefaultTimestamps enables timestamps in output by default.
	DefaultTimestamps = true
)

// options holds configuration for the console adapter.
type options struct {
	// writer sets the output writer.
	// Defaults to os.Stdout.
	writer io.Writer
	// prettyPrint enables or disables pretty printing.
	// When enabled, spans are printed in a human-readable format.
	// When disabled, spans are printed as JSON.
	prettyPrint bool `optgen:"default=DefaultPrettyPrint"`
	// timestamps enables or disables timestamps in output.
	// Only applies when pretty printing is enabled.
	timestamps bool `optgen:"default=DefaultTimestamps"`
}

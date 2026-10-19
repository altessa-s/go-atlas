// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=guardOptions --output=guard_options_gen.go --option-type=GuardOption

// guardOptions configures [TouchGuard]. All fields are populated by generated [GuardOption] setters in guard_options_gen.go.
type guardOptions struct {
	// int64Increment increments the counter by an int64 1 instead of an int32 1, for guard fields other writers keep as int64.
	int64Increment bool
}

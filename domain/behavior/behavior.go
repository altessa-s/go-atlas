// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior

import "strconv"

// Kind enumerates the field-behavior values recognized in `behavior`
// struct tags. The names mirror google.api.field_behavior so the same mental
// model carries across the proto boundary, but the type is independent: this
// package has no protobuf dependency.
type Kind uint8

// Kind values. Unspecified is the zero value and never matches a strip set.
const (
	// Unspecified is the zero Kind; it is produced for unrecognized tag
	// tokens only through error paths and never participates in a strip.
	Unspecified Kind = iota
	// Required marks a field that must be supplied. It carries validation
	// intent rather than a write-side restriction and is not part of any
	// default strip set.
	Required
	// OutputOnly marks a server-set field a client must not supply.
	OutputOnly
	// InputOnly marks a field accepted on input but never returned in a
	// response (passwords, tokens, …).
	InputOnly
	// Immutable marks a field fixed at creation that must not change on update.
	Immutable
	// Identifier marks a resource identifier the server assigns.
	Identifier
)

// tag tokens recognized in `behavior:"…"`, indexed by Kind.
var kindTokens = [...]string{
	Unspecified: "unspecified",
	Required:    "required",
	OutputOnly:  "output_only",
	InputOnly:   "input_only",
	Immutable:   "immutable",
	Identifier:  "identifier",
}

// String returns the snake_case tag token for b, or "kind(N)" for an
// out-of-range value.
func (b Kind) String() string {
	if int(b) < len(kindTokens) {
		return kindTokens[b]
	}
	return "kind(" + strconv.Itoa(int(b)) + ")"
}

// ParseKind maps a tag token (for example "output_only") to its Kind.
// The second result is false for an unrecognized token.
func ParseKind(token string) (Kind, bool) {
	for b, name := range kindTokens {
		if b == int(Unspecified) {
			continue
		}
		if name == token {
			return Kind(b), true
		}
	}
	return Unspecified, false
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sealbox

import "errors"

var (
	// ErrNoKey reports a box without a configured key (a nil [Box]).
	ErrNoKey = errors.New("sealbox: no secret key is configured")
	// ErrUnknownKey reports a sealed value whose key is not configured (it was rotated out).
	ErrUnknownKey = errors.New("sealbox: the value was sealed with an unknown key")
	// ErrOpen reports a value that does not decrypt (tampered, wrong associated data, malformed nonce).
	ErrOpen = errors.New("sealbox: the value does not decrypt")
)

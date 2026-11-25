// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retryqueuefields

import (
	"errors"
	"strings"
)

// Document fields owned by the retry queue storage.
const (
	ID          = "_id"
	Since       = "since"
	Attempts    = "attempts"
	NextAttempt = "nextAttemptAt"
	State       = "state"
	ExpiredAt   = "expiredAt"
	ResolvedAt  = "resolvedAt"
	Generation  = "generation"
)

// ErrInvalidFlagField reports an unusable name for a flag or its reason.
var ErrInvalidFlagField = errors.New("retryqueue mongo: invalid flag field name")

// Reserved reports whether a field belongs to the queue.
func Reserved(name string) bool {
	switch name {
	case ID, Since, Attempts, NextAttempt, State, ExpiredAt, ResolvedAt, Generation:
		return true
	}
	return false
}

// ValidFlag reports whether name identifies a user-defined BSON field distinct
// from its companion, after applying the store's option whitespace trimming.
func ValidFlag(name, companion string) bool {
	name = strings.TrimSpace(name)
	return name != "" && name != strings.TrimSpace(companion) && !Reserved(name) &&
		!strings.HasPrefix(name, "$") && !strings.ContainsAny(name, ".\x00")
}

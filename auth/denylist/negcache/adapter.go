// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package negcache

import (
	"context"

	"github.com/altessa-s/go-atlas/auth/denylist"
)

// FromChecker adapts a synchronous [denylist.Checker] to the ctx-aware
// [Authoritative] interface, so any Checker — the in-process [denylist.Denylist]
// today, a distributed one behind the same seam later — can be the exact tier a
// [Cache] confirms against. The wrapped Checker performs no I/O, so the context
// is unused and no error is ever returned.
func FromChecker(c denylist.Checker) Authoritative {
	return checkerAuthoritative{checker: c}
}

type checkerAuthoritative struct {
	checker denylist.Checker
}

func (a checkerAuthoritative) IsRevoked(_ context.Context, key string) (bool, error) {
	return a.checker.IsRevoked(key), nil
}

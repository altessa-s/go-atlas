// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"slices"
)

// sqlStatePattern is a well-formed SQLSTATE: a two-character class and a three-character subclass of digits and upper case letters.
var sqlStatePattern = regexp.MustCompile(`^[0-9A-Z]{5}$`)

// safeSentinels are the sentinels a [SafeError] result matches with errors.Is, by identity, when the original error's chain holds
// them: cancellation and the database/sql connection states callers branch on.
var safeSentinels = []error{context.Canceled, context.DeadlineExceeded, sql.ErrConnDone, sql.ErrTxDone, driver.ErrBadConn}

// SQLState returns the SQLSTATE of the first error in err's chain that carries one through a SQLState method (pgx, lib/pq), or ""
// when none does or the value is not a well-formed five-character code: the method is the driver's, and its result may be any text.
func SQLState(err error) string {
	e, ok := errors.AsType[interface {
		error
		SQLState() string
	}](err)
	if !ok {
		return ""
	}
	if state := e.SQLState(); sqlStatePattern.MatchString(state) {
		return state
	}
	return ""
}

// SQLStates returns the distinct validated codes from all branches of a joined or wrapped error.
func SQLStates(err error) []string {
	var states []string
	var visit func(error)
	visit = func(err error) {
		for err != nil {
			if e, ok := err.(interface{ SQLState() string }); ok {
				if state := e.SQLState(); sqlStatePattern.MatchString(state) && !slices.Contains(states, state) {
					states = append(states, state)
				}
			}
			if joined, ok := err.(interface{ Unwrap() []error }); ok {
				for _, cause := range joined.Unwrap() {
					visit(cause)
				}
				return
			}
			err = errors.Unwrap(err)
		}
	}
	visit(err)
	return states
}

// SafeError returns an error reporting that op failed with err, without err's text: a driver or connector error may quote the
// connection string, password included, in forms no pattern recognizes reliably. Its text is "<op> failed (<type of err>)", with
// ", SQLSTATE <code>" when err carries a well-formed one ([SQLState]), which its SQLState method also returns, so the classifiers of
// store/postgres still work.
//
// It does not unwrap, and errors.Is matches only kind and the sentinels context.Canceled, context.DeadlineExceeded, sql.ErrConnDone,
// sql.ErrTxDone and driver.ErrBadConn, by identity in err's chain: the original error's own Is methods are never called, so a crafted
// error cannot be probed through it. kind may be nil. A nil err returns nil.
func SafeError(kind error, op string, err error) error {
	if err == nil {
		return nil
	}
	e := &safeError{kind: kind, state: SQLState(err)}
	e.msg = fmt.Sprintf("%s failed (%T)", op, err)
	if e.state != "" {
		e.msg = fmt.Sprintf("%s failed (%T, SQLSTATE %s)", op, err, e.state)
	}
	for _, s := range safeSentinels {
		if chainHolds(err, s) {
			e.sentinels = append(e.sentinels, s)
		}
	}
	return e
}

// safeError hides the original error behind msg; see [SafeError]. It keeps no reference to the original error.
type safeError struct {
	msg       string
	kind      error
	state     string
	sentinels []error
}

func (e *safeError) Error() string { return e.msg }

// SQLState returns the validated SQLSTATE of the original error, or "".
func (e *safeError) SQLState() string { return e.state }

// Is reports whether target is kind or one of the enumerated sentinels the original error's chain held.
func (e *safeError) Is(target error) bool {
	if e.kind != nil && target == e.kind { //nolint:errorlint // identity on purpose
		return true
	}
	return slices.Contains(e.sentinels, target) // == comparison: identity on purpose
}

// chainHolds reports whether target is in err's chain, comparing by identity and following Unwrap only, never an Is method.
func chainHolds(err, target error) bool {
	for err != nil {
		if err == target { //nolint:errorlint // identity on purpose: custom Is methods must not be consulted
			return true
		}
		switch u := err.(type) { //nolint:errorlint // walking the chain by hand on purpose
		case interface{ Unwrap() []error }:
			for _, inner := range u.Unwrap() {
				if chainHolds(inner, target) {
					return true
				}
			}
			return false
		case interface{ Unwrap() error }:
			err = u.Unwrap()
		default:
			return false
		}
	}
	return false
}

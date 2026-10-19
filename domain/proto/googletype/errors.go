// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package googletype

import (
	"errors"
	"strconv"
)

// Conversion errors. A decoder wraps one of them with the offending value;
// classify with errors.Is. They carry no API field path: the caller maps
// them onto its own request fields.
var (
	// ErrImpossibleDate reports a google.type.Date that is neither absent
	// nor a full calendar date in years 1–9999.
	ErrImpossibleDate = errors.New("googletype: impossible date")
	// ErrUnknownWeekday reports an UNSPECIFIED or undefined
	// google.type.DayOfWeek.
	ErrUnknownWeekday = errors.New("googletype: unknown or unspecified day of the week")
	// ErrInvalidDuration reports a google.protobuf.Duration that fails
	// durationpb.CheckValid.
	ErrInvalidDuration = errors.New("googletype: invalid duration")
	// ErrInvalidNanos reports google.type.Money nanos outside ±999,999,999.
	ErrInvalidNanos = errors.New("googletype: money nanos out of range")
	// ErrSignMismatch reports google.type.Money units and nanos of opposite
	// signs.
	ErrSignMismatch = errors.New("googletype: money units and nanos have opposite signs")
	// ErrMissingCurrency reports a google.type.Money without a currency
	// code.
	ErrMissingCurrency = errors.New("googletype: money currency code is empty")
	// ErrUnknownCurrency reports a google.type.Money currency code that is
	// not an ISO 4217 code.
	ErrUnknownCurrency = errors.New("googletype: unknown ISO 4217 currency code")
	// ErrPrecision reports an amount with more than nine fractional digits,
	// which google.type.Money cannot hold.
	ErrPrecision = errors.New("googletype: amount has more than nine fractional digits")
	// ErrOverflow reports an amount whose integer part does not fit in
	// google.type.Money units (int64).
	ErrOverflow = errors.New("googletype: amount does not fit google.type.Money units")
)

// IndexError reports the element of a repeated field a list decoder
// rejected, so the caller can name it ("field[i]") on its own path.
type IndexError struct {
	// Index is the zero-based position of the rejected element.
	Index int
	// Err is the element's conversion error.
	Err error
}

// Error implements error.
func (e *IndexError) Error() string {
	return "element " + strconv.Itoa(e.Index) + ": " + e.Err.Error()
}

// Unwrap returns the element's conversion error.
func (e *IndexError) Unwrap() error { return e.Err }

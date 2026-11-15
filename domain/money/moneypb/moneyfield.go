// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package moneypb

import (
	"errors"

	"github.com/altessa-s/go-atlas/domain/money"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	gtypemoney "google.golang.org/genproto/googleapis/type/money"
)

// Field-error reasons of the decoders that take an API field path. They are
// part of the API contract: a client sees them in the BadRequest violation.
const (
	ReasonInvalidNanos    = "must be within ±999999999"
	ReasonSignMismatch    = "units and nanos must have the same sign"
	ReasonRequired        = "required"
	ReasonUnknownCurrency = "unknown ISO 4217 currency"
	ReasonNegative        = "must not be negative"
)

// MoneyFromProtoAt is [MoneyFromProto] for the API field path field. A
// structural violation is a [coreerrs.FieldError] that keeps the decoder's
// sentinel in its chain: nanos out of range on "field.nanos", units and nanos
// of opposite signs on field, an empty or unknown currency code on
// "field.currencyCode" (the sub-fields use the proto JSON names).
func MoneyFromProtoAt(field string, p *gtypemoney.Money) (money.Money, error) {
	m, err := MoneyFromProto(p)
	if err != nil {
		return money.Money{}, moneyFieldError(field, err)
	}
	return m, nil
}

// NonNegativeMoneyFromProtoAt is [MoneyFromProtoAt] that also rejects a
// negative amount with a [coreerrs.FieldError] on field ("must not be
// negative"). A nil p is the zero Money (absent).
func NonNegativeMoneyFromProtoAt(field string, p *gtypemoney.Money) (money.Money, error) {
	m, err := MoneyFromProtoAt(field, p)
	switch {
	case err != nil:
		return money.Money{}, err
	case m.IsNegative():
		return money.Money{}, coreerrs.NewFieldError(field, ReasonNegative)
	}
	return m, nil
}

// moneyFieldError maps a [MoneyFromProto] error at field to a
// [coreerrs.FieldError] that keeps err in its chain.
func moneyFieldError(field string, err error) error {
	path, reason := field, err.Error()
	switch {
	case errors.Is(err, ErrInvalidNanos):
		path, reason = field+".nanos", ReasonInvalidNanos
	case errors.Is(err, ErrSignMismatch):
		reason = ReasonSignMismatch
	case errors.Is(err, ErrMissingCurrency):
		path, reason = field+".currencyCode", ReasonRequired
	case errors.Is(err, ErrUnknownCurrency):
		path, reason = field+".currencyCode", ReasonUnknownCurrency
	}
	return &coreerrs.FieldError{Field: path, Reason: reason, Err: err}
}

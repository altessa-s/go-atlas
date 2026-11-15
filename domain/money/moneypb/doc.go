// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package moneypb converts between [money.Money] and google.type.Money.
//
// [MoneyFromProto] and [MoneyToProto] convert exactly and fail with sentinel
// errors ([ErrInvalidNanos], [ErrUnknownCurrency], [ErrPrecision], ...);
// [MoneyToProtoFitted] rounds instead of failing. [MoneyFromProtoAt] and
// [NonNegativeMoneyFromProtoAt] report bad input as a go-atlas-core
// FieldError on the API field path, ready for a BadRequest violation.
//
//	m, err := moneypb.MoneyFromProtoAt("order.total", req.GetOrder().GetTotal())
package moneypb

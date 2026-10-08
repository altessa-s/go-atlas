// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package money provides an amount of a currency with exact decimal
// arithmetic, explicit rounding and lossless allocation.
//
// # Amounts
//
// [Money] pairs a shopspring/decimal amount with a [Currency], whose minor
// units come from ISO 4217 ([ParseCurrency]) or are chosen for a custom unit
// ([NewCurrency]). Amounts of different currencies are never combined:
// [Money.Add], [Money.Sub] and [Money.Cmp] fail with [ErrCurrencyMismatch].
//
// # Rounding
//
// Arithmetic never rounds: [Money.Mul] keeps every decimal place. Round to
// the currency's minor units with an explicit [RoundingMode] — [HalfEven]
// (banker's), [HalfUp], [Down], [Up], [Floor] or [Ceiling].
//
// # Allocation
//
// [Money.Allocate] splits an amount by ratios and [Money.Split] into equal
// parts; the parts always sum to the amount, the leftover minor units going
// to the largest remainders.
//
//	total, _ := money.Parse("100.00", "EUR")
//	parts, _ := total.Split(3) // 33.34, 33.33, 33.33
//	vat := total.Mul(decimal.RequireFromString("0.2")).Round(money.HalfEven)
package money

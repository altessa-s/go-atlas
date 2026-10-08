// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package money

import (
	"fmt"
	"strings"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

// MaxMinorUnits bounds the minor units of a [Currency] created with
// [NewCurrency].
const MaxMinorUnits = 18

// Currency is a currency code with the number of its minor units (decimal
// places): 2 for EUR, 0 for JPY, 3 for KWD. The zero Currency is invalid.
type Currency struct {
	code       string
	minorUnits int32
}

// Code returns the currency code, e.g. "EUR".
func (c Currency) Code() string { return c.code }

// MinorUnits returns the number of decimal places of the currency.
func (c Currency) MinorUnits() int32 { return c.minorUnits }

// IsZero reports whether c is the zero, invalid Currency.
func (c Currency) IsZero() bool { return c.code == "" }

// String returns the currency code.
func (c Currency) String() string { return c.code }

// ParseCurrency returns the ISO 4217 currency with the given alphabetic
// code (case-insensitive), or [ErrUnknownCurrency].
func ParseCurrency(code string) (Currency, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	units, ok := iso4217.Get(code)
	if !ok {
		return Currency{}, fmt.Errorf("%w: %q", ErrUnknownCurrency, code)
	}
	return Currency{code: code, minorUnits: units}, nil
}

// MustParseCurrency is [ParseCurrency] that panics on an unknown code, for
// package-level constants.
func MustParseCurrency(code string) Currency {
	c, err := ParseCurrency(code)
	if err != nil {
		panic(err)
	}
	return c
}

// NewCurrency returns a currency outside ISO 4217 — a token, a loyalty
// point, a finer internal unit — with minorUnits decimal places (0 to
// MaxMinorUnits). The code must be non-empty.
func NewCurrency(code string, minorUnits int32) (Currency, error) {
	code = strings.TrimSpace(code)
	if code == "" || minorUnits < 0 || minorUnits > MaxMinorUnits {
		return Currency{}, fmt.Errorf("%w: %q with %d minor units", ErrInvalidCurrency, code, minorUnits)
	}
	return Currency{code: code, minorUnits: minorUnits}, nil
}

// iso4217 maps the active ISO 4217 alphabetic codes to their minor units.
var iso4217 = coremaps.NewImmutableMap(func() map[string]int32 {
	m := make(map[string]int32, 180)
	for _, group := range []struct {
		units int32
		codes string
	}{
		{units: zeroPlaces, codes: iso4217Zero},
		{units: twoPlaces, codes: iso4217Two},
		{units: threePlaces, codes: iso4217Three},
		{units: fourPlaces, codes: iso4217Four},
	} {
		for code := range strings.FieldsSeq(group.codes) {
			m[code] = group.units
		}
	}
	return m
}())

// Minor units of the ISO 4217 groups.
const (
	zeroPlaces int32 = iota
	_
	twoPlaces
	threePlaces
	fourPlaces
)

// Active ISO 4217 codes by minor units.
const (
	iso4217Zero  = `BIF CLP DJF GNF ISK JPY KMF KRW PYG RWF UGX UYI VND VUV XAF XOF XPF`
	iso4217Three = `BHD IQD JOD KWD LYD OMR TND`
	iso4217Four  = `CLF UYW`
)

// iso4217Two lists the active codes with two minor units.
const iso4217Two = `AED AFN ALL AMD AOA ARS AUD AWG AZN BAM BBD BDT BMD BND BOB BOV BRL BSD BTN BWP BYN BZD
CAD CDF CHE CHF CHW CNY COP COU CRC CUP CVE CZK DKK DOP DZD EGP ERN ETB EUR FJD FKP GBP GEL GHS GIP GMD GTQ GYD
HKD HNL HTG HUF IDR ILS INR IRR JMD KES KGS KHR KPW KYD KZT LAK LBP LKR LRD LSL MAD MDL MGA MKD MMK MNT MOP MRU
MUR MVR MWK MXN MXV MYR MZN NAD NGN NIO NOK NPR NZD PAB PEN PGK PHP PKR PLN QAR RON RSD RUB SAR SBD SCR SDG SEK
SGD SHP SLE SOS SRD SSP STN SVC SYP SZL THB TJS TMT TOP TRY TTD TWD TZS UAH USD USN UYU UZS VES VED WST XAD XCD XCG
YER ZAR ZMW ZWG`

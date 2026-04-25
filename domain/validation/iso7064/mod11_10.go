// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package iso7064

import (
	"errors"
	"strconv"
)

// errInvalidNumber is returned by [Mod11_10] when the input is not a valid
// 9-digit numeric string (wrong length or non-digit characters).
var errInvalidNumber = errors.New("invalid number")

const (
	mod11_10Len            = 9
	mod11_10DataDigits     = 8
	mod11_10InitialProduct = 10
	mod11_10Mod10          = 10
	mod11_10Mod11          = 11
	mod11_10Factor         = 2
	mod11_10SumZero        = 10
	mod11_10ValidCheck     = 1
)

// Mod11_10 validates an ISO 7064 MOD 11-10 check digit for a 9-digit number.
// Returns true if valid, false otherwise. Returns error if input is not a valid 9-digit number.
//
// Note: When using int64 input, leading zeros are not preserved. If your identifier can have leading
// zeros, pass it as a string.
//
// Example:
//
//	valid, err := iso7064.Mod11_10("123456788") // true, nil
func Mod11_10[T interface{ string | int64 }](data T) (bool, error) {
	var num string

	switch t := any(data).(type) {
	case int64:
		num = strconv.FormatInt(t, 10)
	default:
		num = t.(string) // nolint: errcheck
	}

	if len(num) != mod11_10Len {
		return false, errInvalidNumber
	}

	product := int64(mod11_10InitialProduct)
	sum := int64(0)

	b := []byte(num)
	for i := range mod11_10DataDigits {
		ch := b[i]
		if ch < '0' || ch > '9' {
			return false, errInvalidNumber
		}
		n := int64(ch - '0')

		if sum = (n + product) % mod11_10Mod10; sum == 0 {
			sum = mod11_10SumZero
		}

		product = (mod11_10Factor * sum) % mod11_10Mod11
	}

	lastCh := b[mod11_10DataDigits]
	if lastCh < '0' || lastCh > '9' {
		return false, errInvalidNumber
	}
	lastDigit := int64(lastCh - '0')

	checkDigit := (product + lastDigit) % mod11_10Mod10

	return checkDigit == mod11_10ValidCheck, nil
}

// IsValidMod11_10 checks if the ISO 7064 MOD 11-10 check digit is valid.
// Returns false for invalid input or failed validation.
//
// Example:
//
//	iso7064.IsValidMod11_10("123456788") // true
func IsValidMod11_10[T interface{ string | int64 }](data T) bool {
	ok, err := Mod11_10(data)
	if err != nil {
		return false
	}
	return ok
}

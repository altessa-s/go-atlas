// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package iso7064_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/validation/iso7064"
)

func TestMod11_10_Examples(t *testing.T) {
	ok, err := iso7064.Mod11_10("123456788")
	require.NoError(t, err)
	require.True(t, ok)

	require.True(t, iso7064.IsValidMod11_10("123456788"))
}

func TestMod11_10_InvalidInput(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "short", in: "123"},
		{name: "long", in: "1234567890"},
		{name: "non_digit", in: "12345678x"},
		{name: "unicode_digits", in: "１２３４５６７８９"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ok, err := iso7064.Mod11_10(tc.in)
			require.Error(t, err)
			require.False(t, ok)
		})
	}
}

func TestMod11_10_Int64_NegativeIsInvalid(t *testing.T) {
	ok, err := iso7064.Mod11_10(int64(-1))
	require.Error(t, err)
	require.False(t, ok)
}

func TestMod11_10_StringLeadingZeros_Accepted(t *testing.T) {
	// This test asserts the input format is accepted and evaluated.
	// (Leading zeros cannot be represented via int64.)
	ok, err := iso7064.Mod11_10("012345678")
	require.NoError(t, err)
	_ = ok // may be true or false depending on check digit; format must be accepted.
}

func TestMod11_10_GeneratedCheckDigits_AreValid(t *testing.T) {
	// For any 8 digits, there is exactly one check digit d in [0..9] such that:
	// (product + d) % 10 == 1.
	//
	// This test ensures the implementation accepts a variety of valid numbers.
	seeds := []string{
		"00000000",
		"11111111",
		"12345678",
		"87654321",
		"99999999",
	}

	for _, data8 := range seeds {
		// Private constants from implementation copied here for testing logic verification
		// mod11_10InitialProduct = 10
		// mod11_10Mod10 = 10
		// mod11_10Mod11 = 11
		// mod11_10Factor = 2
		// mod11_10SumZero = 10
		// mod11_10ValidCheck = 1
		// Data Digits = 8

		product := int64(10)
		sum := int64(0)

		for i := range 8 {
			n := int64(data8[i] - '0')
			if sum = (n + product) % 10; sum == 0 {
				sum = 10
			}
			product = (2 * sum) % 11
		}

		checkDigit := (1 - (product % 10) + 10) % 10
		full := data8 + string(byte('0'+checkDigit))

		ok, err := iso7064.Mod11_10(full)
		require.NoError(t, err)
		require.True(t, ok, "Mod11_10(%q)", full)
	}
}

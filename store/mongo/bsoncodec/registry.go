// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec

import (
	"cloud.google.com/go/civil"
	"go.mongodb.org/mongo-driver/v2/bson"

	mongoxcodec "github.com/altessa-s/go-mongox/bsoncodec"
)

// NewRegistry returns a private registry with driver defaults and codecs for
// Optional, decimal, and both the Google and go-atlas-core civil.Date types.
// Configure additional codecs before using the registry concurrently.
func NewRegistry() *bson.Registry {
	reg := mongoxcodec.NewRegistry()
	if err := mongoxcodec.RegisterDateType[civil.Date](reg); err != nil {
		panic(err) // civil.Date has the required calendar fields.
	}
	return reg
}

// ErrInvalidDate is returned when encoding or decoding an invalid calendar date.
var ErrInvalidDate = mongoxcodec.ErrInvalidDate

// ErrNotADateType is returned by [RegisterDateType] for a type without the
// calendar-date fields.
var ErrNotADateType = mongoxcodec.ErrNotADateType

// ErrDecimalOutOfRange is returned when a decimal's coefficient exceeds 4096
// bits or the value cannot be stored exactly as BSON Decimal128.
var ErrDecimalOutOfRange = mongoxcodec.ErrDecimalOutOfRange

// ErrDecimalNotFinite is returned when a stored Decimal128 is NaN or infinity.
var ErrDecimalNotFinite = mongoxcodec.ErrDecimalNotFinite

// RegisterDateType registers on reg the calendar-date codec for T: a struct
// with the fields Year int, Month time.Month and Day int, such as
// cloud.google.com/go/civil.Date (registered by [NewRegistry]) or a
// service's own date type. A date is stored as its "YYYY-MM-DD" string —
// nothing can shift it across a day boundary, and the strings sort in date
// order. The zero date is stored as null, or as "" with
// [WithDateZeroAsEmptyString]; both decode to the zero date. An invalid date
// or a year outside 0–9999 fails with [ErrInvalidDate], as does one that T
// rejects through a Valid() bool method; a BSON date is
// rejected, since reading an instant as a calendar date would pick a zone.
// Pointers to T are handled by the driver's pointer codec (nil <-> null).
// Registration also admits T as a scalar keyset filter value on this registry.
// Register before the registry is used concurrently.
func RegisterDateType[T any](reg *bson.Registry, opts ...DateOption) error {
	if newDateOptions(opts...).zeroAsEmptyString {
		return mongoxcodec.RegisterDateType[T](reg, mongoxcodec.WithDateZeroAsEmptyString())
	}
	return mongoxcodec.RegisterDateType[T](reg)
}

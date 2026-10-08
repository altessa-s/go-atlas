// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec

import (
	"errors"
	"fmt"
	"reflect"

	"cloud.google.com/go/civil"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ErrInvalidDate is returned when a civil.Date that is not a valid calendar
// date is encoded, or a stored string is not a YYYY-MM-DD date.
var ErrInvalidDate = errors.New("bsoncodec: invalid civil date")

var civilDateType = reflect.TypeFor[civil.Date]()

// registerCivil installs the civil.Date codec. A date is stored as its
// "YYYY-MM-DD" string: it has no time or zone to shift, and strings of that
// form sort in date order. The zero Date is stored as null. Pointers are
// handled by the driver's pointer codec (nil <-> null).
func registerCivil(reg *bson.Registry) {
	reg.RegisterTypeEncoder(civilDateType, bson.ValueEncoderFunc(encodeCivilDate))
	reg.RegisterTypeDecoder(civilDateType, bson.ValueDecoderFunc(decodeCivilDate))
}

func encodeCivilDate(_ bson.EncodeContext, vw bson.ValueWriter, v reflect.Value) error {
	if !v.IsValid() || v.Type() != civilDateType {
		return bson.ValueEncoderError{Name: "CivilDateEncodeValue", Types: []reflect.Type{civilDateType}, Received: v}
	}
	d, _ := v.Interface().(civil.Date)
	if d.IsZero() {
		return vw.WriteNull()
	}
	// Years outside 0..9999 have no four-digit form and would not sort.
	if !d.IsValid() || d.Year < 0 || d.Year > 9999 {
		return fmt.Errorf("%w: %04d-%02d-%02d", ErrInvalidDate, d.Year, d.Month, d.Day)
	}
	return vw.WriteString(d.String())
}

func decodeCivilDate(_ bson.DecodeContext, vr bson.ValueReader, v reflect.Value) error {
	if !v.CanSet() || v.Type() != civilDateType {
		return bson.ValueDecoderError{Name: "CivilDateDecodeValue", Types: []reflect.Type{civilDateType}, Received: v}
	}
	switch t := vr.Type(); t {
	case bson.TypeNull:
		if err := vr.ReadNull(); err != nil {
			return err
		}
		v.Set(reflect.ValueOf(civil.Date{}))
		return nil
	case bson.TypeString:
		s, err := vr.ReadString()
		if err != nil {
			return err
		}
		d, err := civil.ParseDate(s)
		if err != nil || len(s) != len("2006-01-02") {
			return fmt.Errorf("%w: %q", ErrInvalidDate, s)
		}
		v.Set(reflect.ValueOf(d))
		return nil
	default:
		// A BSON date is an instant; reading it as a calendar date would
		// pick a zone silently.
		return fmt.Errorf("bsoncodec: cannot decode %v into a civil.Date", t)
	}
}

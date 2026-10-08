// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ErrDecimalOutOfRange is returned when a decimal.Decimal cannot be stored
// exactly as a BSON Decimal128: more than 34 significant digits, or an
// exponent outside [-6176, 6111]. The value is never rounded.
var ErrDecimalOutOfRange = errors.New("bsoncodec: decimal not representable as Decimal128")

// ErrDecimalNotFinite is returned when a stored Decimal128 is NaN or an
// infinity, which decimal.Decimal cannot hold.
var ErrDecimalNotFinite = errors.New("bsoncodec: Decimal128 is not a finite number")

var (
	decimalType     = reflect.TypeFor[decimal.Decimal]()
	nullDecimalType = reflect.TypeFor[decimal.NullDecimal]()
)

// registerDecimal installs the decimal.Decimal and decimal.NullDecimal codecs.
// Pointers to either are handled by the driver's pointer codec (nil <-> null).
func registerDecimal(reg *bson.Registry) {
	reg.RegisterTypeEncoder(decimalType, bson.ValueEncoderFunc(encodeDecimal))
	reg.RegisterTypeDecoder(decimalType, bson.ValueDecoderFunc(decodeDecimal))
	reg.RegisterTypeEncoder(nullDecimalType, bson.ValueEncoderFunc(encodeNullDecimal))
	reg.RegisterTypeDecoder(nullDecimalType, bson.ValueDecoderFunc(decodeNullDecimal))
}

// toDecimal128 converts d exactly, or fails with [ErrDecimalOutOfRange].
func toDecimal128(d decimal.Decimal) (bson.Decimal128, error) {
	d128, ok := bson.ParseDecimal128FromBigInt(d.Coefficient(), int(d.Exponent()))
	if !ok {
		// d.String() would expand an extreme exponent into billions of
		// digits; report the shape instead.
		return bson.Decimal128{}, fmt.Errorf("%w: %d-bit coefficient, exponent %d",
			ErrDecimalOutOfRange, d.Coefficient().BitLen(), d.Exponent())
	}
	return d128, nil
}

// fromDecimal128 converts d128, or fails with [ErrDecimalNotFinite].
func fromDecimal128(d128 bson.Decimal128) (decimal.Decimal, error) {
	coefficient, exp, err := d128.BigInt()
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf("%w: %s", ErrDecimalNotFinite, d128.String())
	}
	return decimal.NewFromBigInt(coefficient, int32(exp)), nil //nolint:gosec // Decimal128 exponents fit in int32
}

func encodeDecimal(_ bson.EncodeContext, vw bson.ValueWriter, v reflect.Value) error {
	if !v.IsValid() || v.Type() != decimalType {
		return bson.ValueEncoderError{Name: "DecimalEncodeValue", Types: []reflect.Type{decimalType}, Received: v}
	}
	d, _ := v.Interface().(decimal.Decimal)
	d128, err := toDecimal128(d)
	if err != nil {
		return err
	}
	return vw.WriteDecimal128(d128)
}

func encodeNullDecimal(ec bson.EncodeContext, vw bson.ValueWriter, v reflect.Value) error {
	if !v.IsValid() || v.Type() != nullDecimalType {
		return bson.ValueEncoderError{Name: "NullDecimalEncodeValue", Types: []reflect.Type{nullDecimalType}, Received: v}
	}
	nd, _ := v.Interface().(decimal.NullDecimal)
	if !nd.Valid {
		return vw.WriteNull()
	}
	return encodeDecimal(ec, vw, reflect.ValueOf(nd.Decimal))
}

// readDecimal reads a Decimal128, or an exact integer (Int32/Int64), as a
// decimal. A double is rejected rather than converted, so no binary
// floating-point error enters a decimal field.
func readDecimal(vr bson.ValueReader) (decimal.Decimal, error) {
	switch t := vr.Type(); t {
	case bson.TypeDecimal128:
		d128, err := vr.ReadDecimal128()
		if err != nil {
			return decimal.Decimal{}, err
		}
		return fromDecimal128(d128)
	case bson.TypeInt32:
		i, err := vr.ReadInt32()
		if err != nil {
			return decimal.Decimal{}, err
		}
		return decimal.NewFromInt32(i), nil
	case bson.TypeInt64:
		i, err := vr.ReadInt64()
		if err != nil {
			return decimal.Decimal{}, err
		}
		return decimal.NewFromInt(i), nil
	default:
		return decimal.Decimal{}, fmt.Errorf("bsoncodec: cannot decode %v into a decimal", t)
	}
}

func decodeDecimal(_ bson.DecodeContext, vr bson.ValueReader, v reflect.Value) error {
	if !v.CanSet() || v.Type() != decimalType {
		return bson.ValueDecoderError{Name: "DecimalDecodeValue", Types: []reflect.Type{decimalType}, Received: v}
	}
	if vr.Type() == bson.TypeNull {
		if err := vr.ReadNull(); err != nil {
			return err
		}
		v.Set(reflect.ValueOf(decimal.Decimal{}))
		return nil
	}
	d, err := readDecimal(vr)
	if err != nil {
		return err
	}
	v.Set(reflect.ValueOf(d))
	return nil
}

func decodeNullDecimal(_ bson.DecodeContext, vr bson.ValueReader, v reflect.Value) error {
	if !v.CanSet() || v.Type() != nullDecimalType {
		return bson.ValueDecoderError{Name: "NullDecimalDecodeValue", Types: []reflect.Type{nullDecimalType}, Received: v}
	}
	if vr.Type() == bson.TypeNull {
		if err := vr.ReadNull(); err != nil {
			return err
		}
		v.Set(reflect.ValueOf(decimal.NullDecimal{}))
		return nil
	}
	d, err := readDecimal(vr)
	if err != nil {
		return err
	}
	v.Set(reflect.ValueOf(decimal.NullDecimal{Decimal: d, Valid: true}))
	return nil
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package optional

import (
	"bytes"
	"encoding/json"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// jsonNull is the canonical JSON encoding of None. It is returned
// directly from MarshalJSON, so callers must not modify the returned
// slice (encoding/json never does).
var jsonNull = []byte("null")

// IsZero reports whether the Optional is None. It lets struct fields
// tagged with `bson:",omitempty"` (and other libraries that consult
// IsZero) skip None values on the wire.
func (o Optional[T]) IsZero() bool {
	return !o.present
}

// MarshalBSONValue encodes the Optional as a single BSON value. Some
// values are emitted as their underlying T (so Some(time.Now()) becomes
// a BSON datetime, Some("") becomes a BSON empty string, etc.), and
// None is emitted as BSON null.
//
// Combined with IsZero, a struct field tagged `bson:"field,omitempty"`
// will omit the field entirely when the Optional is None.
func (o Optional[T]) MarshalBSONValue() (byte, []byte, error) {
	if !o.present {
		return byte(bson.TypeNull), nil, nil
	}
	typ, data, err := bson.MarshalValue(o.value)
	if err != nil {
		return 0, nil, err
	}
	return byte(typ), data, nil
}

// UnmarshalBSONValue decodes a single BSON value into the Optional.
// BSON null becomes None; any other type is decoded into T and stored
// as Some. A decode failure leaves the Optional unchanged.
func (o *Optional[T]) UnmarshalBSONValue(typ byte, data []byte) error {
	if bson.Type(typ) == bson.TypeNull {
		var zero T
		o.value = zero
		o.present = false
		return nil
	}
	var v T
	if err := bson.UnmarshalValue(bson.Type(typ), data, &v); err != nil {
		return err
	}
	o.value = v
	o.present = true
	return nil
}

// MarshalJSON encodes the Optional as JSON. Some(v) is encoded as v's
// JSON form; None is encoded as the JSON literal null.
//
// To omit None fields entirely, use a struct field type of
// *Optional[T] (then a None Optional can be omitted by setting the
// pointer to nil), or rely on the bson omitempty + IsZero pairing on
// the BSON side. Standard encoding/json does not consult IsZero.
func (o Optional[T]) MarshalJSON() ([]byte, error) {
	if !o.present {
		return jsonNull, nil
	}
	return json.Marshal(o.value)
}

// UnmarshalJSON decodes JSON into the Optional. The JSON literal null
// becomes None; any other input is decoded into T and stored as Some.
// An absent field never invokes UnmarshalJSON, so the zero value of
// Optional[T] (None) is the natural default.
func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, jsonNull) {
		var zero T
		o.value = zero
		o.present = false
		return nil
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	o.value = v
	o.present = true
	return nil
}

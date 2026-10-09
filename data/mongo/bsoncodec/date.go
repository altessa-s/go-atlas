// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"cloud.google.com/go/civil"
	"go.mongodb.org/mongo-driver/v2/bson"

	atlascivil "github.com/altessa-s/go-atlas/core/time/civil"
)

// ErrInvalidDate is returned when a date that is not a valid calendar date
// in years 0–9999 is encoded, or a stored string is not a YYYY-MM-DD date.
var ErrInvalidDate = errors.New("bsoncodec: invalid civil date")

// ErrNotADateType is returned by [RegisterDateType] for a type without the
// calendar-date fields.
var ErrNotADateType = errors.New("bsoncodec: not a calendar date type")

// dateLayout is the stored form; its lexical order is the date order.
const dateLayout = "2006-01-02"

// maxDateYear is the last year with a four-digit stored form.
const maxDateYear = 9999

var (
	intType   = reflect.TypeFor[int]()
	monthType = reflect.TypeFor[time.Month]()
)

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
// Register before the registry is used concurrently.
func RegisterDateType[T any](reg *bson.Registry, opts ...DateOption) error {
	t := reflect.TypeFor[T]()
	fields, err := dateFieldsOf(t)
	if err != nil {
		return err
	}
	c := &dateCodec{t: t, fields: fields, opts: newDateOptions(opts...), validates: reflect.PointerTo(t).Implements(reflect.TypeFor[dateValidator]())}
	reg.RegisterTypeEncoder(t, bson.ValueEncoderFunc(c.encode))
	reg.RegisterTypeDecoder(t, bson.ValueDecoderFunc(c.decode))
	return nil
}

// dateFields holds the field indexes of Year, Month and Day.
type dateFields struct{ year, month, day int }

// dateFieldsOf checks that t is a struct with the calendar-date fields.
func dateFieldsOf(t reflect.Type) (dateFields, error) {
	if t.Kind() != reflect.Struct {
		return dateFields{}, fmt.Errorf("%w: %v is not a struct", ErrNotADateType, t)
	}
	index := func(name string, want reflect.Type) (int, error) {
		f, ok := t.FieldByName(name)
		if !ok || len(f.Index) != 1 || !f.IsExported() || f.Type != want {
			return 0, fmt.Errorf("%w: %v needs an exported field %s %v", ErrNotADateType, t, name, want)
		}
		return f.Index[0], nil
	}
	var (
		f   dateFields
		err error
	)
	if f.year, err = index("Year", intType); err != nil {
		return dateFields{}, err
	}
	if f.month, err = index("Month", monthType); err != nil {
		return dateFields{}, err
	}
	if f.day, err = index("Day", intType); err != nil {
		return dateFields{}, err
	}
	return f, nil
}

// dateCodec encodes and decodes one registered date type.
type dateCodec struct {
	t      reflect.Type
	fields dateFields
	opts   *dateOptions
	// validates is set when T or *T has a Valid() bool method, which then
	// also bounds the dates the codec writes and reads.
	validates bool
}

// dateValidator is a date type with its own validity rule.
type dateValidator interface{ Valid() bool }

func (c *dateCodec) encode(_ bson.EncodeContext, vw bson.ValueWriter, v reflect.Value) error {
	if !v.IsValid() || v.Type() != c.t {
		return bson.ValueEncoderError{Name: "DateEncodeValue", Types: []reflect.Type{c.t}, Received: v}
	}
	year := int(v.Field(c.fields.year).Int())
	month := time.Month(v.Field(c.fields.month).Int())
	day := int(v.Field(c.fields.day).Int())
	if year == 0 && month == 0 && day == 0 {
		if c.opts.zeroAsEmptyString {
			return vw.WriteString("")
		}
		return vw.WriteNull()
	}
	if !validDate(year, month, day) || !c.typeValid(v) {
		return fmt.Errorf("%w: %04d-%02d-%02d", ErrInvalidDate, year, int(month), day)
	}
	return vw.WriteString(fmt.Sprintf("%04d-%02d-%02d", year, int(month), day))
}

func (c *dateCodec) decode(_ bson.DecodeContext, vr bson.ValueReader, v reflect.Value) error {
	if !v.CanSet() || v.Type() != c.t {
		return bson.ValueDecoderError{Name: "DateDecodeValue", Types: []reflect.Type{c.t}, Received: v}
	}
	switch t := vr.Type(); t {
	case bson.TypeNull:
		if err := vr.ReadNull(); err != nil {
			return err
		}
		c.set(v, 0, 0, 0)
		return nil
	case bson.TypeString:
		s, err := vr.ReadString()
		if err != nil {
			return err
		}
		if s == "" {
			c.set(v, 0, 0, 0)
			return nil
		}
		parsed, err := time.Parse(dateLayout, s)
		if err != nil || len(s) != len(dateLayout) {
			return fmt.Errorf("%w: %q", ErrInvalidDate, s)
		}
		d := c.date(parsed.Year(), parsed.Month(), parsed.Day())
		if !c.typeValid(d) {
			return fmt.Errorf("%w: %q", ErrInvalidDate, s)
		}
		v.Set(d)
		return nil
	default:
		return fmt.Errorf("bsoncodec: cannot decode %v into a calendar date", t)
	}
}

// set stores the date parts in v, leaving any other fields of T zero.
func (c *dateCodec) set(v reflect.Value, year int, month time.Month, day int) {
	v.Set(c.date(year, month, day))
}

// date builds a T from the date parts, any other fields zero.
func (c *dateCodec) date(year int, month time.Month, day int) reflect.Value {
	d := reflect.New(c.t).Elem()
	d.Field(c.fields.year).SetInt(int64(year))
	d.Field(c.fields.month).SetInt(int64(month))
	d.Field(c.fields.day).SetInt(int64(day))
	return d
}

// typeValid applies T's own Valid() rule to the non-zero date v, when T or
// *T has one; a copy is addressed, so a pointer receiver applies too.
func (c *dateCodec) typeValid(v reflect.Value) bool {
	if !c.validates {
		return true
	}
	p := reflect.New(c.t)
	p.Elem().Set(v)
	dv, _ := p.Interface().(dateValidator)
	return dv.Valid()
}

// validDate reports whether the parts form a real date in years 0–9999.
func validDate(year int, month time.Month, day int) bool {
	if year < 0 || year > maxDateYear || month < time.January || month > time.December || day < 1 {
		return false
	}
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return t.Year() == year && t.Month() == month && t.Day() == day
}

// registerCivil installs the codecs for cloud.google.com/go/civil.Date and
// go-atlas [atlascivil.Date]. The go-atlas date stores its zero value as "",
// the same as its text form.
func registerCivil(reg *bson.Registry) {
	if err := RegisterDateType[civil.Date](reg); err != nil {
		panic(err) // civil.Date has the date fields; unreachable
	}
	if err := RegisterDateType[atlascivil.Date](reg, WithDateZeroAsEmptyString()); err != nil {
		panic(err) // atlascivil.Date has the date fields; unreachable
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec

import (
	"reflect"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/core/types/optional"
)

// NewRegistry returns a private registry with the driver's defaults and codecs
// for every Optional[T]. Configure additional codecs before using the registry
// concurrently. Inner values use this registry, including user-defined codecs.
func NewRegistry() *bson.Registry {
	defaults := bson.NewRegistry()
	// Obtain the standard structural hooks from a separate registry so their
	// lookup cache cannot shadow our replacements in the returned registry.
	enc, _ := defaults.LookupEncoder(reflect.TypeFor[optional.Optional[int]]())
	dec, _ := defaults.LookupDecoder(reflect.TypeFor[optional.Optional[int]]())
	reg := bson.NewRegistry()
	reg.RegisterInterfaceEncoder(
		reflect.TypeFor[bson.ValueMarshaler](),
		bson.ValueEncoderFunc(func(ec bson.EncodeContext, vw bson.ValueWriter, value reflect.Value) error {
			v := value
			if v.Kind() == reflect.Pointer && optional.IsOptionalType(v.Type().Elem()) {
				if v.IsNil() {
					return vw.WriteNull()
				}
				v = v.Elem()
			}
			if !optional.IsOptionalType(v.Type()) {
				return enc.EncodeValue(ec, vw, value)
			}
			inner, present := optional.GetReflect(v)
			if !present {
				return vw.WriteNull()
			}
			encoder, err := ec.LookupEncoder(inner.Type())
			if err != nil {
				return err
			}
			return encoder.EncodeValue(ec, vw, inner)
		}),
	)
	reg.RegisterInterfaceDecoder(
		reflect.TypeFor[bson.ValueUnmarshaler](),
		bson.ValueDecoderFunc(func(dc bson.DecodeContext, vr bson.ValueReader, value reflect.Value) error {
			v := value
			if v.Kind() == reflect.Pointer && optional.IsOptionalType(v.Type().Elem()) {
				if v.IsNil() {
					v.Set(reflect.New(v.Type().Elem()))
				}
				v = v.Elem()
			}
			if !optional.IsOptionalType(v.Type()) {
				return dec.DecodeValue(dc, vr, value)
			}
			if vr.Type() == bson.TypeNull {
				if err := vr.ReadNull(); err != nil {
					return err
				}
				v.Set(optional.NoneReflect(v.Type()))
				return nil
			}
			inner := reflect.New(optional.InnerType(v.Type())).Elem()
			decoder, err := dc.LookupDecoder(inner.Type())
			if err != nil {
				return err
			}
			if err := decoder.DecodeValue(dc, vr, inner); err != nil {
				return err
			}
			v.Set(optional.SomeReflect(v.Type(), inner))
			return nil
		}),
	)
	return reg
}

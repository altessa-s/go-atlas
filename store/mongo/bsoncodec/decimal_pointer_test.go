// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec

import (
	"errors"
	"reflect"
	"strconv"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestDecodeDecimalPointerAssignment(t *testing.T) {
	decodeErr := errors.New("decode failed")
	for _, pointerType := range []reflect.Type{decimalPtrType, nullDecimalPtrType} {
		for _, existing := range []bool{false, true} {
			for _, fail := range []bool{false, true} {
				t.Run(pointerType.String()+"/existing="+strconv.FormatBool(existing)+"/fail="+strconv.FormatBool(fail), func(t *testing.T) {
					value := reflect.New(pointerType).Elem()
					if existing {
						value.Set(reflect.New(pointerType.Elem()))
					}
					before := value.Pointer()
					decoder := func(_ bson.DecodeContext, _ bson.ValueReader, dst reflect.Value) error {
						if dst.Type() != pointerType.Elem() || !dst.CanSet() {
							t.Fatalf("unexpected decode destination: %v", dst)
						}
						if existing && dst.Addr().Pointer() != before {
							t.Fatal("decoder did not reuse the existing pointee")
						}
						if fail {
							return decodeErr
						}
						return nil
					}
					reader := decimalPointerReader{}
					err := decodeDecimalPointerValue(bson.DecodeContext{}, reader, value, pointerType, "test", decoder)
					if fail && !errors.Is(err, decodeErr) || !fail && err != nil {
						t.Fatalf("unexpected decode error: %v", err)
					}
					if existing && value.Pointer() != before {
						t.Fatal("existing pointer was replaced")
					}
					if !existing && value.IsNil() != fail {
						t.Fatal("new pointer must be assigned only on success")
					}
				})
			}
		}
	}
}

// decimalPointerReader supplies a non-null type; the injected decoder does not read a value.
type decimalPointerReader struct {
	bson.ValueReader
}

func (decimalPointerReader) Type() bson.Type { return bson.TypeInt32 }

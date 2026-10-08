// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"bytes"
	"reflect"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// defaultRegistry is the driver's default registry, the baseline
// registryLeaf compares a configured registry against.
var defaultRegistry = sync.OnceValue(bson.NewRegistry)

// registryLeafKey identifies a memoized registryLeaf result.
type registryLeafKey struct {
	reg *bson.Registry
	t   reflect.Type
}

// registryLeaves memoizes registryLeaf per registry and type.
var registryLeaves sync.Map // map[registryLeafKey]bool

// registryLeaf reports whether reg encodes t with another encoder than the
// default registry does — a type encoder registered for t, such as the
// data/mongo/bsoncodec decimal codec in place of the struct codec — so the
// value must be written as a unit, not folded field by field. A nil reg has
// no leaves of its own. See [sameEncoder] for how encoders are compared.
func registryLeaf(reg *bson.Registry, t reflect.Type) bool {
	if reg == nil {
		return false
	}
	key := registryLeafKey{reg: reg, t: t}
	if v, ok := registryLeaves.Load(key); ok {
		b, _ := v.(bool)
		return b
	}
	enc, err := reg.LookupEncoder(t)
	leaf := false
	if err == nil {
		def, defErr := defaultRegistry().LookupEncoder(t)
		leaf = defErr != nil || !sameEncoder(enc, def)
	}
	registryLeaves.Store(key, leaf)
	return leaf
}

// sameEncoder reports whether a and b are the same encoder implementation.
// Encoders of different Go types differ. Function encoders
// (bson.ValueEncoderFunc) are told apart by their code pointer: each registry
// builds its default encoders from the same method values, while a custom
// function — for a named array or slice type, say — has its own code. Codec
// structs (the struct, slice, map codecs) are fresh instances per registry,
// so for them only the type is compared.
func sameEncoder(a, b bson.ValueEncoder) bool {
	if reflect.TypeOf(a) != reflect.TypeOf(b) {
		return false
	}
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	if va.Kind() == reflect.Func {
		return va.Pointer() == vb.Pointer()
	}
	return true
}

// marshal encodes v as a BSON document with reg, or with the default
// registry when reg is nil.
func marshal(v any, reg *bson.Registry) ([]byte, error) {
	if reg == nil {
		return bson.Marshal(v)
	}
	var buf bytes.Buffer
	enc := bson.NewEncoder(bson.NewDocumentWriter(&buf))
	enc.SetRegistry(reg)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

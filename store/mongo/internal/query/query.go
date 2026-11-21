// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package query

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-mongox/bsonfilter"

	"github.com/altessa-s/go-atlas/store/mongo/bsoncodec"

	coremaps "github.com/altessa-s/go-atlas-core/collections/maps"
)

// DefaultTimeout caps read operations.
const DefaultTimeout = 5 * time.Second

// CloseTimeout bounds cursor cleanup independently of caller cancellation.
const CloseTimeout = 5 * time.Second

// maxFilterDepth bounds recursive validation, including cyclic Go values.
const maxFilterDepth = 1000

// Registry is the shared default document encoding registry. Treat it as read-only.
var Registry = sync.OnceValue(bsoncodec.NewRegistry)

// ErrDangerousOperator identifies a filter containing a server-side JavaScript operator.
var ErrDangerousOperator = bsonfilter.ErrDangerousOperator

// dangerousFilterOperators is the deny-list of MongoDB query operators
// that execute server-side JavaScript (or otherwise have a well-known
// abuse path) and therefore must never appear in a filter built from
// untrusted input. The list is intentionally narrow — perf-only hazards
// like $regex stay allowed; only operators that grant attacker-controlled
// code execution on the database are blocked.
var dangerousFilterOperators = coremaps.NewImmutableMap(map[string]struct{}{
	"$where":       {}, // arbitrary JavaScript evaluated server-side
	"$function":    {}, // user-defined JavaScript inside aggregations / $expr
	"$accumulator": {}, // user-defined JavaScript accumulator
})

// ValidateFilter walks a BSON filter and reports
// [ErrDangerousOperator] when any dangerous operator (see
// [dangerousFilterOperators]) is reachable at any depth, including through
// typed slices, raw BSON values, and nested documents.
//
// Query helpers that accept filters from untrusted boundaries MUST run this before passing
// the filter to the driver. The check is O(n) over the filter document
// and runs once per call, before the network round-trip — cost is
// negligible compared to the query itself.
func ValidateFilter(filter any) error {
	return validateFilterValueDepth(filter, 0)
}

func validateFilterDocumentDepth(filter bson.M, depth int) error {
	if depth > maxFilterDepth {
		return fmt.Errorf("mongo: filter nesting exceeds validation limit")
	}
	for key, value := range filter {
		if dangerousFilterOperators.Contains(key) {
			return fmt.Errorf("%w: %s", ErrDangerousOperator, key)
		}
		if err := validateFilterValueDepth(value, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// validateFilterValueDepth recurses into BSON and reflectable Go value shapes.
func validateFilterValueDepth(v any, depth int) error {
	if depth > maxFilterDepth {
		return fmt.Errorf("mongo: filter nesting exceeds validation limit")
	}
	switch v := v.(type) {
	case bson.M:
		return validateFilterDocumentDepth(v, depth+1)
	case bson.D:
		for _, e := range v {
			if dangerousFilterOperators.Contains(e.Key) {
				return fmt.Errorf("%w: %s", ErrDangerousOperator, e.Key)
			}
			if err := validateFilterValueDepth(e.Value, depth+1); err != nil {
				return err
			}
		}
	case bson.A:
		for _, item := range v {
			if err := validateFilterValueDepth(item, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range v {
			if err := validateFilterValueDepth(item, depth+1); err != nil {
				return err
			}
		}
	case []bson.M:
		for _, item := range v {
			if err := validateFilterDocumentDepth(item, depth+1); err != nil {
				return err
			}
		}
	case bson.Raw:
		return validateRawFilterDocument(v, depth+1)
	case bson.RawValue:
		return validateRawFilterValue(v, depth+1)
	default:
		return validateFilterReflect(reflect.ValueOf(v), depth+1)
	}
	return nil
}

// validateRawFilterDocument checks an encoded document without losing nested
// documents stored in typed slices such as []bson.D.
func validateRawFilterDocument(raw bson.Raw, depth int) error {
	if depth > maxFilterDepth {
		return fmt.Errorf("mongo: filter nesting exceeds validation limit")
	}
	elems, err := raw.Elements()
	if err != nil {
		return err
	}
	for _, elem := range elems {
		if dangerousFilterOperators.Contains(elem.Key()) {
			return fmt.Errorf("%w: %s", ErrDangerousOperator, elem.Key())
		}
		if err := validateRawFilterValue(elem.Value(), depth+1); err != nil {
			return err
		}
	}
	return nil
}

func validateRawFilterValue(value bson.RawValue, depth int) error {
	if depth > maxFilterDepth {
		return fmt.Errorf("mongo: filter nesting exceeds validation limit")
	}
	switch value.Type { //nolint:exhaustive // Only BSON documents and arrays contain filter keys.
	case bson.TypeEmbeddedDocument:
		doc, ok := value.DocumentOK()
		if ok {
			return validateRawFilterDocument(doc, depth+1)
		}
	case bson.TypeArray:
		arr, ok := value.ArrayOK()
		if !ok {
			return nil
		}
		items, err := arr.Values()
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := validateRawFilterValue(item, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateFilterReflect handles named maps and typed slices beyond the BSON
// aliases. The depth cap also makes self-referential Go values fail closed.
func validateFilterReflect(v reflect.Value, depth int) error {
	if !v.IsValid() || depth > maxFilterDepth {
		if depth > maxFilterDepth {
			return fmt.Errorf("mongo: filter nesting exceeds validation limit")
		}
		return nil
	}
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		depth++
		if depth > maxFilterDepth {
			return fmt.Errorf("mongo: filter nesting exceeds validation limit")
		}
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	switch v.Kind() { //nolint:exhaustive // Only containers and structs can contain filter keys.
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			key := iter.Key()
			if key.Kind() == reflect.String && dangerousFilterOperators.Contains(key.String()) {
				return fmt.Errorf("%w: %s", ErrDangerousOperator, key.String())
			}
			if err := validateFilterReflect(iter.Value(), depth+1); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := validateFilterReflect(v.Index(i), depth+1); err != nil {
				return err
			}
		}
	case reflect.Struct:
		return validateFilterStruct(v, depth)
	}
	return nil
}

// validateFilterStruct checks BSON elements directly and encodes other structs.
func validateFilterStruct(v reflect.Value, depth int) error {
	if v.Type() == reflect.TypeFor[bson.E]() {
		e := v.Interface().(bson.E) //nolint:errcheck // exact type checked above
		if dangerousFilterOperators.Contains(e.Key) {
			return fmt.Errorf("%w: %s", ErrDangerousOperator, e.Key)
		}
		return validateFilterValueDepth(e.Value, depth+1)
	}
	if v.CanInterface() {
		typ, data, err := bson.MarshalValue(v.Interface())
		if err == nil {
			return validateRawFilterValue(bson.RawValue{Type: typ, Value: data}, depth+1)
		}
	}
	return nil
}

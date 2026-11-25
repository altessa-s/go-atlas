// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package document

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/internal/bsonfield"
)

// ErrEncryptionNotEnabled reports that an encrypted field has no encryptor.
var ErrEncryptionNotEnabled = errors.New("encryption not enabled")

// EncryptFunc encrypts driver-encoded plaintext using the field's algorithm and key.
type EncryptFunc func(context.Context, bson.RawValue, string, string) (bson.Binary, error)

type fieldKey struct {
	owner reflect.Type
	name  string
}

// Fields binds an immutable snapshot of encryption models and struct-tag rules
// to the client's explicit encryptor. BSON names and layout belong to the caller.
type Fields struct {
	tag     string
	models  map[fieldKey]EncryptionField
	encrypt EncryptFunc
}

// New snapshots model rules. A rule must select an exported field declared on
// its model type. Rules for embedded fields belong to their declaring type.
func New(tag string, models map[reflect.Type]*EncryptionModel, encrypt EncryptFunc) (*Fields, error) {
	fields := &Fields{tag: tag, models: make(map[fieldKey]EncryptionField), encrypt: encrypt}
	for typ, model := range models {
		if typ == nil || model == nil {
			return nil, fmt.Errorf("%w: encryption model requires a type", bsonfield.ErrFieldTransform)
		}
		if typ.Kind() != reflect.Struct {
			return nil, fmt.Errorf("%w: model type %s must be a struct", bsonfield.ErrFieldTransform, typ)
		}
		for _, rule := range model.Fields {
			sf, ok := typ.FieldByName(rule.FieldName)
			if !ok || !sf.IsExported() || len(sf.Index) != 1 {
				return nil, fmt.Errorf("%w: %s.%s must be a declared exported field", bsonfield.ErrFieldTransform, typ, rule.FieldName)
			}
			if rule.Algorithm == "" || rule.KeyAltName == "" {
				return nil, fmt.Errorf("%w: missing algorithm or key for %s.%s", bsonfield.ErrFieldTransform, typ, rule.FieldName)
			}
			if _, err := Algorithm(rule.Algorithm); err != nil {
				return nil, fmt.Errorf("%w: %s.%s: %w", bsonfield.ErrFieldTransform, typ, rule.FieldName, err)
			}
			key := fieldKey{typ, rule.FieldName}
			if _, duplicate := fields.models[key]; duplicate {
				return nil, fmt.Errorf("%w: duplicate rule for %s.%s", bsonfield.ErrFieldTransform, typ, rule.FieldName)
			}
			fields.models[key] = rule
		}
	}
	// Inspect after loading every model so cross-model references are independent
	// of map iteration order. Reachable tagged fields also participate.
	for typ := range models {
		if bsonfield.Inspect(typ, fields.Matches) == bsonfield.OverlappingFields {
			return nil, fmt.Errorf("%w: overlapping encryption rules in %s", bsonfield.ErrFieldTransform, typ)
		}
	}
	return fields, nil
}

// Matches reports every declared rule, including a malformed tag, so a bad
// declaration cannot cause a field to bypass Transform.
func (f *Fields) Matches(owner reflect.Type, sf reflect.StructField) bool {
	if _, ok := f.models[fieldKey{owner, sf.Name}]; ok {
		return true
	}
	_, ok := sf.Tag.Lookup(f.tag)
	return ok
}

// Transform resolves the declared Go field through inline ancestors. The
// builder's encoder carries the active registry, minsize and addressability.
func (f *Fields) Transform(ctx context.Context, owner reflect.Type, path []int, name string,
	encode func() (bson.RawValue, error)) (bson.Binary, error) {
	for _, i := range path[:len(path)-1] {
		owner = owner.Field(i).Type
		if owner.Kind() == reflect.Pointer {
			owner = owner.Elem()
		}
	}
	sf := owner.Field(path[len(path)-1])
	rule, ok := f.models[fieldKey{owner, sf.Name}]
	if !ok {
		tag, present := sf.Tag.Lookup(f.tag)
		algorithm, key, complete := strings.Cut(tag, ",")
		if !present || !complete || algorithm == "" || key == "" || strings.Contains(key, ",") {
			return bson.Binary{}, fmt.Errorf("%w: malformed encryption tag for %s.%s (%q)", bsonfield.ErrFieldTransform, owner, sf.Name, name)
		}
		rule = EncryptionField{Algorithm: algorithm, KeyAltName: key}
	}
	if f.encrypt == nil {
		return bson.Binary{}, ErrEncryptionNotEnabled
	}
	raw, err := encode()
	if err != nil {
		return bson.Binary{}, err
	}
	ciphertext, err := f.encrypt(ctx, raw, rule.Algorithm, rule.KeyAltName)
	if err != nil {
		return bson.Binary{}, err
	}
	if ciphertext.Subtype != bson.TypeBinaryEncrypted || len(ciphertext.Data) == 0 {
		return bson.Binary{}, fmt.Errorf("%w: encryptor returned invalid ciphertext for %s.%s", bsonfield.ErrFieldTransform, owner, sf.Name)
	}
	return ciphertext, nil
}

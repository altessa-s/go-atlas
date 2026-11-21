// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsontag_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/internal/bsontag"
)

func TestLookup(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		tag       reflect.StructTag
		key, want string
	}{
		{"bare BSON", "private", "bson", "private"},
		{"bare custom", "private", "db", ""},
		{"custom", `bson:"a" db:"b"`, "db", "b"},
		{"empty BSON", `bson:"" db:"b"`, "bson", ""},
		{"other key", `json:"a"`, "bson", ""},
		{"malformed with colon", "a:b", "bson", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, bsontag.Lookup(tc.tag, tc.key))
		})
	}
}

func TestParse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		tag  string
		want bsontag.Info
	}{
		{"-", bsontag.Info{Skip: true}},
		{"-,omitempty", bsontag.Info{Name: "-", OmitEmpty: true}},
		{",unknown", bsontag.Info{Name: "fieldname"}},
		{"omitonupdate,omitempty,minsize,inline", bsontag.Info{Name: "omitonupdate", OmitOnUpdate: true, OmitEmpty: true, MinSize: true, Inline: true}},
		{"name,omitonupdate", bsontag.Info{Name: "name", OmitOnUpdate: true}},
	} {
		t.Run(tc.tag, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, bsontag.Parse(tc.tag, "FieldName"))
		})
	}
}

func TestDriverParity(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		tag          reflect.StructTag
		inlineStruct bool
	}{
		{"skip", `bson:"-"`, false},
		{"dash key", `bson:"-,omitempty"`, false},
		{"omit empty", `bson:",omitempty"`, false},
		{"omit empty as name", `bson:"omitempty"`, false},
		{"inline as name", `bson:"inline"`, true},
		{"inline unnamed", `bson:",inline"`, true},
		{"inline named", `bson:"name,inline"`, true},
		{"minsize", `bson:"name,minsize"`, false},
		{"bare", "private", false},
		{"absent", "", false},
		{"empty", `bson:""`, false},
		{"Atlas extension", `bson:"name,omitonupdate"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			typ := reflect.TypeFor[int64]()
			if tc.inlineStruct {
				typ = reflect.TypeFor[struct{ Inner int64 }]()
			}
			field := reflect.StructField{Name: "FieldName", Type: typ, Tag: tc.tag}
			value := reflect.New(reflect.StructOf([]reflect.StructField{field})).Elem()
			info := bsontag.Parse(bsontag.Lookup(field.Tag, "bson"), field.Name)
			for _, n := range []int64{0, 42} {
				fv := value.Field(0)
				if tc.inlineStruct {
					fv = fv.Field(0)
				}
				fv.SetInt(n)
				data, err := bson.Marshal(value.Interface())
				require.NoError(t, err)
				var got bson.M
				require.NoError(t, bson.Unmarshal(data, &got))
				want := bson.M{}
				switch {
				case info.Skip || (info.OmitEmpty && n == 0):
				case info.Inline:
					want["inner"] = n
				case tc.inlineStruct:
					want[info.Name] = bson.D{{Key: "inner", Value: n}}
				case info.MinSize:
					want[info.Name] = int32(n)
				default:
					want[info.Name] = n
				}
				require.Equal(t, want, got)
			}
		})
	}
}

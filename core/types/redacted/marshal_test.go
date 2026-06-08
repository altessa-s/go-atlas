// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redacted_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"gopkg.in/yaml.v3"

	"github.com/altessa-s/go-atlas/core/types/redacted"
)

// --- JSON ------------------------------------------------------------------

func TestRedactedString_MarshalJSON(t *testing.T) {
	t.Parallel()

	s := redacted.RedactedString(plain)
	data, err := json.Marshal(s)
	require.NoError(t, err)

	var decoded string
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, expected, decoded)
	require.NotContains(t, string(data), plain)
}

func TestRedactedString_MarshalJSON_InStruct(t *testing.T) {
	t.Parallel()

	type cfg struct {
		Password redacted.RedactedString `json:"password"`
	}
	data, err := json.Marshal(cfg{Password: plain})
	require.NoError(t, err)

	var decoded map[string]string
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, expected, decoded["password"])
	require.NotContains(t, string(data), plain)
}

func TestRedactedString_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	var s redacted.RedactedString
	require.NoError(t, json.Unmarshal([]byte(`"`+plain+`"`), &s))
	require.Equal(t, plain, s.Expose())
}

func TestRedactedString_UnmarshalJSON_Invalid(t *testing.T) {
	t.Parallel()

	var s redacted.RedactedString
	err := json.Unmarshal([]byte(`{"not":"a string"}`), &s)
	require.Error(t, err)
}

func TestRedactedString_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	type cfg struct {
		Token redacted.RedactedString `json:"token"`
	}

	var c cfg
	require.NoError(t, json.Unmarshal([]byte(`{"token":"`+plain+`"}`), &c))
	require.Equal(t, plain, c.Token.Expose())

	data, err := json.Marshal(c)
	require.NoError(t, err)

	// json.Marshal HTML-escapes < and >; decode to verify the on-wire string.
	var decoded map[string]string
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, expected, decoded["token"])
	require.NotContains(t, string(data), plain)
}

// --- YAML ------------------------------------------------------------------

func TestRedactedString_MarshalYAML(t *testing.T) {
	t.Parallel()

	s := redacted.RedactedString(plain)
	data, err := yaml.Marshal(s)
	require.NoError(t, err)
	require.Equal(t, expected+"\n", string(data))
}

func TestRedactedString_UnmarshalYAML(t *testing.T) {
	t.Parallel()

	var s redacted.RedactedString
	require.NoError(t, yaml.Unmarshal([]byte(plain+"\n"), &s))
	require.Equal(t, plain, s.Expose())
}

func TestRedactedString_YAMLRoundTrip(t *testing.T) {
	t.Parallel()

	type cfg struct {
		Token redacted.RedactedString `yaml:"token"`
	}

	var c cfg
	require.NoError(t, yaml.Unmarshal([]byte("token: "+plain+"\n"), &c))
	require.Equal(t, plain, c.Token.Expose())

	data, err := yaml.Marshal(c)
	require.NoError(t, err)
	require.Contains(t, string(data), expected)
	require.NotContains(t, string(data), plain)
}

// --- Text ------------------------------------------------------------------

func TestRedactedString_MarshalText(t *testing.T) {
	t.Parallel()

	data, err := redacted.RedactedString(plain).MarshalText()
	require.NoError(t, err)
	require.Equal(t, expected, string(data))
}

func TestRedactedString_UnmarshalText(t *testing.T) {
	t.Parallel()

	var s redacted.RedactedString
	require.NoError(t, s.UnmarshalText([]byte(plain)))
	require.Equal(t, plain, s.Expose())
}

// --- BSON ------------------------------------------------------------------

func TestRedactedString_MarshalBSONValue(t *testing.T) {
	t.Parallel()

	typ, data, err := redacted.RedactedString(plain).MarshalBSONValue()
	require.NoError(t, err)
	require.Equal(t, byte(bson.TypeString), typ)

	var decoded string
	require.NoError(t, bson.UnmarshalValue(bson.Type(typ), data, &decoded))
	require.Equal(t, expected, decoded)
}

func TestRedactedString_MarshalBSON_InStruct(t *testing.T) {
	t.Parallel()

	type doc struct {
		Password redacted.RedactedString `bson:"password"`
	}
	data, err := bson.Marshal(doc{Password: plain})
	require.NoError(t, err)

	var decoded map[string]string
	require.NoError(t, bson.Unmarshal(data, &decoded))
	require.Equal(t, expected, decoded["password"])
}

func TestRedactedString_BSON_OmitEmpty(t *testing.T) {
	t.Parallel()

	type doc struct {
		Password redacted.RedactedString `bson:"password,omitempty"`
	}
	data, err := bson.Marshal(doc{})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, bson.Unmarshal(data, &decoded))
	_, present := decoded["password"]
	require.False(t, present, "empty RedactedString must be omitted via IsZero")
}

func TestRedactedString_UnmarshalBSONValue(t *testing.T) {
	t.Parallel()

	typ, data, err := bson.MarshalValue(plain)
	require.NoError(t, err)

	var s redacted.RedactedString
	require.NoError(t, s.UnmarshalBSONValue(byte(typ), data))
	require.Equal(t, plain, s.Expose())
}

func TestRedactedString_BSONRoundTrip(t *testing.T) {
	t.Parallel()

	type doc struct {
		Token redacted.RedactedString `bson:"token"`
	}

	raw, err := bson.Marshal(map[string]string{"token": plain})
	require.NoError(t, err)

	var d doc
	require.NoError(t, bson.Unmarshal(raw, &d))
	require.Equal(t, plain, d.Token.Expose())

	out, err := bson.Marshal(d)
	require.NoError(t, err)

	var decoded map[string]string
	require.NoError(t, bson.Unmarshal(out, &decoded))
	require.Equal(t, expected, decoded["token"])
}

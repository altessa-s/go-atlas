// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSecret_String(t *testing.T) {
	s := Secret("super-secret")
	assert.Equal(t, "<redacted>", s.String())
	assert.Equal(t, "<redacted>", fmt.Sprintf("%s", s))
	assert.Equal(t, "<redacted>", fmt.Sprintf("%v", s))
}

func TestSecret_GoString(t *testing.T) {
	s := Secret("super-secret")
	assert.Equal(t, "Secret{<redacted>}", s.GoString())
	assert.Equal(t, "Secret{<redacted>}", fmt.Sprintf("%#v", s))
}

func TestSecret_MarshalJSON(t *testing.T) {
	s := Secret("super-secret")
	data, err := json.Marshal(s)
	require.NoError(t, err)

	// json.Marshal HTML-escapes <> by default; verify round-trip value.
	var decoded string
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "<redacted>", decoded)
	assert.NotContains(t, string(data), "super-secret")
}

func TestSecret_MarshalJSON_InStruct(t *testing.T) {
	type cfg struct {
		Password Secret `json:"password"`
	}
	data, err := json.Marshal(cfg{Password: "super-secret"})
	require.NoError(t, err)

	var decoded map[string]string
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "<redacted>", decoded["password"])
	assert.NotContains(t, string(data), "super-secret")
}

func TestSecret_MarshalYAML(t *testing.T) {
	s := Secret("super-secret")
	data, err := yaml.Marshal(s)
	require.NoError(t, err)
	assert.Equal(t, "<redacted>\n", string(data))
}

func TestSecret_MarshalText(t *testing.T) {
	s := Secret("super-secret")
	data, err := s.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "<redacted>", string(data))
}

func TestSecret_LogValue(t *testing.T) {
	s := Secret("super-secret")
	val := s.LogValue()
	assert.Equal(t, slog.StringValue("<redacted>"), val)
}

func TestSecret_Expose(t *testing.T) {
	s := Secret("super-secret")
	assert.Equal(t, "super-secret", s.Expose())
}

func TestSecret_IsEmpty(t *testing.T) {
	assert.True(t, Secret("").IsEmpty())
	assert.False(t, Secret("x").IsEmpty())
}

func TestSecret_SecureString(t *testing.T) {
	s := Secret("super-secret")
	ss := s.SecureString()
	require.NotNil(t, ss)
	assert.Equal(t, "super-secret", ss.String())
	ss.Clear()
}

func TestSecret_YAMLRoundTrip(t *testing.T) {
	type cfg struct {
		Token Secret `yaml:"token"`
	}

	// Unmarshal: YAML string → Secret field
	var c cfg
	err := yaml.Unmarshal([]byte("token: my-token\n"), &c)
	require.NoError(t, err)
	assert.Equal(t, "my-token", c.Token.Expose())

	// Marshal: Secret field → redacted YAML
	data, err := yaml.Marshal(c)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<redacted>")
	assert.NotContains(t, string(data), "my-token")
}

func TestSecret_ZeroValue(t *testing.T) {
	var s Secret
	assert.True(t, s.IsEmpty())
	assert.Equal(t, "", s.Expose())
	assert.Equal(t, "<redacted>", s.String())
}

func TestSecret_Comparison(t *testing.T) {
	// Untyped string constants can be compared with Secret.
	s := Secret("value")
	assert.True(t, s != "")
	assert.True(t, Secret("") == "")
}

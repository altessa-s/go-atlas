// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redacted_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/altessa-s/go-atlas/core/types/redacted"
)

const urlSecret = "s3cr3t-pa55"

func TestURL_Masking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"no credentials", "http://proxy:3128", "http://proxy:3128"},
		{"socks5 no credentials", "socks5h://10.0.0.1:1080", "socks5h://10.0.0.1:1080"},
		{"user and password", "http://svc:" + urlSecret + "@proxy:3128", "http://svc:xxxxx@proxy:3128"},
		{"username only", "socks5://svc@proxy:1080", "socks5://svc@proxy:1080"},
		{"malformed", "http://svc:" + urlSecret + "@proxy example:3128/%zz", "<redacted>"},
		{"no scheme with credentials", "svc:" + urlSecret + "@proxy:3128", "<redacted>"},
		{"unencoded slash in password", "http://svc:prefix@" + urlSecret + "/remaining@proxy:3128", "<redacted>"},
		{"second at in path", "http://svc:pw@proxy:3128/a@" + urlSecret, "<redacted>"},
		{"at in query without user info", "http://proxy:3128/?" + urlSecret + "@x", "<redacted>"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			u := redacted.URL(tc.raw)
			require.Equal(t, tc.raw, u.Expose(), "Expose returns the plain text")
			require.Equal(t, tc.want, u.String())
		})
	}
}

func TestURL_HasAmbiguousUserInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want bool
	}{
		{"", false},
		{"http://proxy:3128", false},
		{"http://svc:pw@proxy:3128", false},
		{"socks5://svc@proxy:1080", false},
		{"http://svc:prefix@secret/remaining@proxy:3128", true},
		{"http://svc:prefix@secret:1/remaining@proxy:3128", true},
		{"svc:pw@proxy:3128", true},
		{"http://proxy:3128/?a@b", true},
		{"http://svc:pw@proxy example:3128/%zz", true},
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, redacted.URL(tc.raw).HasAmbiguousUserInfo())
		})
	}
}

func TestURL_OutputPathsNeverLeak(t *testing.T) {
	t.Parallel()

	type config struct {
		URL redacted.URL `yaml:"url" json:"url"`
	}

	for _, raw := range []string{
		"http://svc:" + urlSecret + "@proxy:3128",
		"http://svc:" + urlSecret + "@proxy example:3128/%zz",
		"svc:" + urlSecret + "@proxy:3128",
		"http://svc:prefix@" + urlSecret + "/remaining@proxy:3128",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			c := config{URL: redacted.URL(raw)}

			jsonOut, err := json.Marshal(c)
			require.NoError(t, err)
			yamlOut, err := yaml.Marshal(c)
			require.NoError(t, err)
			textOut, err := c.URL.MarshalText()
			require.NoError(t, err)

			var logged bytes.Buffer
			slog.New(slog.NewJSONHandler(&logged, nil)).Info("cfg", slog.Any("cfg", c), slog.Any("url", c.URL))
			var text bytes.Buffer
			slog.New(slog.NewTextHandler(&text, nil)).Info("cfg", slog.Any("url", c.URL))

			for name, out := range map[string]string{
				"fmt %v":    fmt.Sprintf("%v", c),
				"fmt %+v":   fmt.Sprintf("%+v", c),
				"fmt %#v":   fmt.Sprintf("%#v", c),
				"fmt %s":    fmt.Sprintf("%s", c.URL),
				"fmt %q":    fmt.Sprintf("%q", c.URL),
				"json":      string(jsonOut),
				"yaml":      string(yamlOut),
				"text":      string(textOut),
				"slog json": logged.String(),
				"slog text": text.String(),
			} {
				require.NotContains(t, out, urlSecret, "%s output leaks the password", name)
			}
		})
	}
}

func TestURL_UnmarshalRoundTrip(t *testing.T) {
	t.Parallel()

	raw := "http://svc:" + urlSecret + "@proxy:3128"

	var fromYAML struct {
		URL redacted.URL `yaml:"url"`
	}
	require.NoError(t, yaml.Unmarshal([]byte("url: "+raw+"\n"), &fromYAML))
	require.Equal(t, raw, fromYAML.URL.Expose())

	var fromJSON redacted.URL
	require.NoError(t, json.Unmarshal([]byte(`"`+raw+`"`), &fromJSON))
	require.Equal(t, raw, fromJSON.Expose())
	require.Error(t, json.Unmarshal([]byte(`42`), &fromJSON))

	var fromText redacted.URL
	require.NoError(t, fromText.UnmarshalText([]byte(raw)))
	require.Equal(t, raw, fromText.Expose())

	require.True(t, redacted.URL("").IsEmpty())
	require.True(t, redacted.URL("").IsZero())
	require.False(t, fromText.IsZero())
}

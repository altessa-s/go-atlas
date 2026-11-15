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

	"github.com/altessa-s/go-atlas-core/types/redacted"
)

const urlSecret = "s3cr3t-pa55"

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

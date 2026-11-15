// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pluginsconfig_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pluginsconfig "github.com/altessa-s/go-atlas/config/plugins"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func pluginsFieldErrors(t *testing.T, err error) validation.Errors {
	t.Helper()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	return verrs
}

func TestConfig_IsEnabled(t *testing.T) {
	t.Parallel()

	var nilCfg *pluginsconfig.Config
	require.False(t, nilCfg.IsEnabled())

	c := pluginsconfig.Default()
	require.False(t, c.IsEnabled())
	c.Enabled = true
	require.True(t, c.IsEnabled())
	require.NoError(t, c.Validate())
}

func TestConfig_Validate_Enabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		mut   func(*pluginsconfig.Config)
		field string
	}{
		{"empty dir", func(c *pluginsconfig.Config) { c.Dir = "" }, "Dir"},
		{"init timeout below 1ms", func(c *pluginsconfig.Config) { c.InitTimeout = time.Microsecond }, "InitTimeout"},
		{"watch debounce below 1ms", func(c *pluginsconfig.Config) {
			c.Watch = true
			c.WatchDebounce = time.Microsecond
		}, "WatchDebounce"},
		{"signature without key", func(c *pluginsconfig.Config) { c.Signature.Mode = "require" }, "Signature"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := pluginsconfig.Default()
			c.Enabled = true
			tc.mut(&c)
			require.Contains(t, pluginsFieldErrors(t, c.Validate()), tc.field)
		})
	}

	disabled := pluginsconfig.Config{}
	require.NoError(t, disabled.Validate(), "disabled plugins are not validated")
}

func TestSignature_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&pluginsconfig.Signature{}).Validate(), "empty mode disables verification")
	require.NoError(t, (&pluginsconfig.Signature{Mode: "warn", PublicKeyPath: "/etc/key.pem"}).Validate())

	verrs := pluginsFieldErrors(t, (&pluginsconfig.Signature{Mode: "strict"}).Validate())
	require.Contains(t, verrs, "Mode")
	require.Contains(t, verrs, "PublicKeyPath")
}

func TestCapabilities_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&pluginsconfig.Capabilities{Keep: []string{"CAP_BOGUS"}}).Validate(), "disabled skips checks")
	require.NoError(t, (&pluginsconfig.Capabilities{Enabled: true}).Validate(), "empty keep drops everything")
	require.NoError(t, (&pluginsconfig.Capabilities{Enabled: true, Keep: []string{"cap_net_bind_service"}}).Validate())

	require.Contains(t, pluginsFieldErrors(t, (&pluginsconfig.Capabilities{
		Enabled: true, Keep: []string{"CAP_BOGUS"},
	}).Validate()), "Keep")
	require.Contains(t, pluginsFieldErrors(t, (&pluginsconfig.Capabilities{
		Enabled: true, Keep: []string{""},
	}).Validate()), "Keep")
}

func TestSandbox_Validate_Nested(t *testing.T) {
	t.Parallel()

	s := pluginsconfig.Sandbox{Enabled: true}
	s.Capabilities = pluginsconfig.Capabilities{Enabled: true, Keep: []string{"CAP_BOGUS"}}
	require.Contains(t, pluginsFieldErrors(t, s.Validate()), "Capabilities")

	c := pluginsconfig.Default()
	c.Enabled = true
	c.Sandbox = s
	require.Contains(t, pluginsFieldErrors(t, c.Validate()), "Sandbox")
}

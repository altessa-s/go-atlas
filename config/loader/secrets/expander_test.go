// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader/secrets"

	secsecrets "github.com/altessa-s/go-atlas/security/secrets"
)

// mockManager implements secrets.Manager for testing.
type mockManager struct {
	values map[string]string
	err    error
}

func (m *mockManager) Value(_ context.Context, key string, _ bool) (*secsecrets.Value[string], error) {
	if m.err != nil {
		return nil, m.err
	}
	v, ok := m.values[key]
	if !ok {
		return nil, secsecrets.ErrNotFound
	}
	return &secsecrets.Value[string]{Key: key, Value: v}, nil
}

func newMockManager(vals map[string]string) *mockManager {
	return &mockManager{values: vals}
}

func TestHasSecrets(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"no secrets", "plain text", false},
		{"empty string", "", false},
		{"single secret", "$__secret{ns:key}", true},
		{"secret in text", "prefix $__secret{ns:key} suffix", true},
		{"multiple secrets", "$__secret{a:b} and $__secret{c:d}", true},
		{"incomplete pattern", "$__secret{nocolon}", false},
		{"missing closing brace", "$__secret{ns:key", false},
		{"wrong prefix", "$__sec{ns:key}", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, secrets.HasSecrets(tt.content))
		})
	}
}

func TestExpander_Expand(t *testing.T) {
	mgr := newMockManager(map[string]string{
		"app:password":  "s3cret",
		"db:connection": "host=localhost",
	})

	expander := secrets.New(mgr)
	ctx := t.Context()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"no placeholders", "plain text", "plain text"},
		{"single placeholder", "$__secret{app:password}", "s3cret"},
		{"placeholder in text", "pass=$__secret{app:password}!", "pass=s3cret!"},
		{"multiple placeholders", "$__secret{app:password} $__secret{db:connection}", "s3cret host=localhost"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expander.Expand(ctx, tt.content)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	t.Run("missing secret returns error (fail-closed)", func(t *testing.T) {
		_, err := expander.Expand(ctx, "$__secret{app:missing}")
		require.Error(t, err)
	})
}

func TestExpander_Expand_NilManager(t *testing.T) {
	expander := secrets.New(nil)
	got, err := expander.Expand(t.Context(), "prefix $__secret{ns:key} suffix")
	require.NoError(t, err)
	require.Equal(t, "prefix  suffix", got)
}

func TestExpandString(t *testing.T) {
	mgr := newMockManager(map[string]string{"app:key": "value"})
	ctx := t.Context()

	t.Run("with secrets", func(t *testing.T) {
		got, err := secrets.ExpandString(ctx, "$__secret{app:key}", mgr)
		require.NoError(t, err)
		require.Equal(t, "value", got)
	})

	t.Run("nil manager", func(t *testing.T) {
		got, err := secrets.ExpandString(ctx, "$__secret{app:key}", nil)
		require.NoError(t, err)
		require.Equal(t, "$__secret{app:key}", got)
	})

	t.Run("no secrets", func(t *testing.T) {
		got, err := secrets.ExpandString(ctx, "plain", mgr)
		require.NoError(t, err)
		require.Equal(t, "plain", got)
	})
}

func TestExpandEnvValue(t *testing.T) {
	mgr := newMockManager(map[string]string{"app:key": "value"})
	ctx := t.Context()

	t.Run("with secrets", func(t *testing.T) {
		got, err := secrets.ExpandEnvValue(ctx, "$__secret{app:key}", mgr)
		require.NoError(t, err)
		require.Equal(t, "value", got)
	})

	t.Run("nil manager", func(t *testing.T) {
		got, err := secrets.ExpandEnvValue(ctx, "$__secret{app:key}", nil)
		require.NoError(t, err)
		require.Equal(t, "$__secret{app:key}", got)
	})

	t.Run("no secrets", func(t *testing.T) {
		got, err := secrets.ExpandEnvValue(ctx, "plain", mgr)
		require.NoError(t, err)
		require.Equal(t, "plain", got)
	})

	t.Run("missing secret returns error", func(t *testing.T) {
		_, err := secrets.ExpandEnvValue(ctx, "$__secret{app:missing}", mgr)
		require.Error(t, err)
	})
}

func TestExpandStruct(t *testing.T) {
	mgr := newMockManager(map[string]string{
		"app:password": "s3cret",
		"app:host":     "localhost",
	})
	ctx := t.Context()

	t.Run("string fields", func(t *testing.T) {
		type Config struct {
			Password string
			Host     string
			Plain    string
		}
		cfg := &Config{
			Password: "$__secret{app:password}",
			Host:     "$__secret{app:host}",
			Plain:    "no-secret",
		}
		require.NoError(t, secrets.ExpandStruct(ctx, cfg, mgr))
		require.Equal(t, "s3cret", cfg.Password)
		require.Equal(t, "localhost", cfg.Host)
		require.Equal(t, "no-secret", cfg.Plain)
	})

	t.Run("pointer string field", func(t *testing.T) {
		type Config struct {
			Password *string
		}
		pw := "$__secret{app:password}"
		cfg := &Config{Password: &pw}
		require.NoError(t, secrets.ExpandStruct(ctx, cfg, mgr))
		require.Equal(t, "s3cret", *cfg.Password)
	})

	t.Run("nested struct", func(t *testing.T) {
		type DB struct {
			Password string
		}
		type Config struct {
			Database DB
		}
		cfg := &Config{Database: DB{Password: "$__secret{app:password}"}}
		require.NoError(t, secrets.ExpandStruct(ctx, cfg, mgr))
		require.Equal(t, "s3cret", cfg.Database.Password)
	})

	t.Run("string slice", func(t *testing.T) {
		type Config struct {
			Items []string
		}
		cfg := &Config{Items: []string{"$__secret{app:password}", "plain"}}
		require.NoError(t, secrets.ExpandStruct(ctx, cfg, mgr))
		require.Equal(t, "s3cret", cfg.Items[0])
		require.Equal(t, "plain", cfg.Items[1])
	})

	t.Run("map string values", func(t *testing.T) {
		type Config struct {
			Env map[string]string
		}
		cfg := &Config{Env: map[string]string{
			"PASS": "$__secret{app:password}",
			"HOST": "plain",
		}}
		require.NoError(t, secrets.ExpandStruct(ctx, cfg, mgr))
		require.Equal(t, "s3cret", cfg.Env["PASS"])
		require.Equal(t, "plain", cfg.Env["HOST"])
	})

	t.Run("nil manager", func(t *testing.T) {
		type Config struct{ V string }
		cfg := &Config{V: "$__secret{app:password}"}
		require.NoError(t, secrets.ExpandStruct(ctx, cfg, nil))
		require.Equal(t, "$__secret{app:password}", cfg.V)
	})

	t.Run("not a pointer", func(t *testing.T) {
		type Config struct{ V string }
		cfg := Config{}
		require.Error(t, secrets.ExpandStruct(ctx, cfg, mgr))
	})

	t.Run("nil input", func(t *testing.T) {
		require.NoError(t, secrets.ExpandStruct(ctx, nil, mgr))
	})
}

func TestExpander_Expand_ManagerError(t *testing.T) {
	mgr := &mockManager{err: errors.New("connection failed")}
	expander := secrets.New(mgr)

	_, err := expander.Expand(t.Context(), "val=$__secret{app:key}")
	require.Error(t, err)
}

func TestExpander_Expand_FailOnErrorDisabled(t *testing.T) {
	mgr := newMockManager(map[string]string{
		"app:password": "s3cret",
	})
	expander := secrets.New(mgr, secrets.WithFailOnError(false))
	ctx := t.Context()

	t.Run("missing secret returns empty string", func(t *testing.T) {
		got, err := expander.Expand(ctx, "$__secret{app:missing}")
		require.NoError(t, err)
		require.Equal(t, "", got)
	})

	t.Run("manager error returns empty string", func(t *testing.T) {
		errMgr := &mockManager{err: errors.New("connection failed")}
		failOpenExpander := secrets.New(errMgr, secrets.WithFailOnError(false))
		got, err := failOpenExpander.Expand(ctx, "val=$__secret{app:key}")
		require.NoError(t, err)
		require.Equal(t, "val=", got)
	})

	t.Run("existing secret still resolves", func(t *testing.T) {
		got, err := expander.Expand(ctx, "$__secret{app:password}")
		require.NoError(t, err)
		require.Equal(t, "s3cret", got)
	})
}

func TestExpander_Expand_FailClosed_MultipleSecrets(t *testing.T) {
	mgr := newMockManager(map[string]string{
		"app:password": "s3cret",
	})
	expander := secrets.New(mgr)

	// First secret resolves but second one doesn't — should fail on the first failure
	_, err := expander.Expand(t.Context(), "$__secret{app:password} $__secret{app:missing}")
	require.Error(t, err)
}

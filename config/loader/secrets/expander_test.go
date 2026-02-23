// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"context"
	"errors"
	"testing"

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
			if got := secrets.HasSecrets(tt.content); got != tt.want {
				t.Errorf("HasSecrets(%q) = %v, want %v", tt.content, got, tt.want)
			}
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
			if err != nil {
				t.Fatalf("Expand() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Expand(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}

	t.Run("missing secret returns error (fail-closed)", func(t *testing.T) {
		_, err := expander.Expand(ctx, "$__secret{app:missing}")
		if err == nil {
			t.Fatal("Expand() expected error for missing secret, got nil")
		}
	})
}

func TestExpander_Expand_NilManager(t *testing.T) {
	expander := secrets.New(nil)
	got, err := expander.Expand(t.Context(), "prefix $__secret{ns:key} suffix")
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	if got != "prefix  suffix" {
		t.Errorf("Expand() = %q, want %q", got, "prefix  suffix")
	}
}

func TestExpandString(t *testing.T) {
	mgr := newMockManager(map[string]string{"app:key": "value"})
	ctx := t.Context()

	t.Run("with secrets", func(t *testing.T) {
		got, err := secrets.ExpandString(ctx, "$__secret{app:key}", mgr)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if got != "value" {
			t.Errorf("got %q, want %q", got, "value")
		}
	})

	t.Run("nil manager", func(t *testing.T) {
		got, err := secrets.ExpandString(ctx, "$__secret{app:key}", nil)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if got != "$__secret{app:key}" {
			t.Errorf("got %q, want original", got)
		}
	})

	t.Run("no secrets", func(t *testing.T) {
		got, err := secrets.ExpandString(ctx, "plain", mgr)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if got != "plain" {
			t.Errorf("got %q, want %q", got, "plain")
		}
	})
}

func TestExpandEnvValue(t *testing.T) {
	mgr := newMockManager(map[string]string{"app:key": "value"})
	ctx := t.Context()

	t.Run("with secrets", func(t *testing.T) {
		got, err := secrets.ExpandEnvValue(ctx, "$__secret{app:key}", mgr)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if got != "value" {
			t.Errorf("got %q, want %q", got, "value")
		}
	})

	t.Run("nil manager", func(t *testing.T) {
		got, err := secrets.ExpandEnvValue(ctx, "$__secret{app:key}", nil)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if got != "$__secret{app:key}" {
			t.Errorf("got %q, want original", got)
		}
	})

	t.Run("no secrets", func(t *testing.T) {
		got, err := secrets.ExpandEnvValue(ctx, "plain", mgr)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if got != "plain" {
			t.Errorf("got %q, want %q", got, "plain")
		}
	})

	t.Run("missing secret returns error", func(t *testing.T) {
		_, err := secrets.ExpandEnvValue(ctx, "$__secret{app:missing}", mgr)
		if err == nil {
			t.Fatal("expected error for missing secret, got nil")
		}
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
		if err := secrets.ExpandStruct(ctx, cfg, mgr); err != nil {
			t.Fatal(err)
		}
		if cfg.Password != "s3cret" {
			t.Errorf("Password = %q, want %q", cfg.Password, "s3cret")
		}
		if cfg.Host != "localhost" {
			t.Errorf("Host = %q, want %q", cfg.Host, "localhost")
		}
		if cfg.Plain != "no-secret" {
			t.Errorf("Plain = %q, want %q", cfg.Plain, "no-secret")
		}
	})

	t.Run("pointer string field", func(t *testing.T) {
		type Config struct {
			Password *string
		}
		pw := "$__secret{app:password}"
		cfg := &Config{Password: &pw}
		if err := secrets.ExpandStruct(ctx, cfg, mgr); err != nil {
			t.Fatal(err)
		}
		if *cfg.Password != "s3cret" {
			t.Errorf("Password = %q, want %q", *cfg.Password, "s3cret")
		}
	})

	t.Run("nested struct", func(t *testing.T) {
		type DB struct {
			Password string
		}
		type Config struct {
			Database DB
		}
		cfg := &Config{Database: DB{Password: "$__secret{app:password}"}}
		if err := secrets.ExpandStruct(ctx, cfg, mgr); err != nil {
			t.Fatal(err)
		}
		if cfg.Database.Password != "s3cret" {
			t.Errorf("Database.Password = %q, want %q", cfg.Database.Password, "s3cret")
		}
	})

	t.Run("string slice", func(t *testing.T) {
		type Config struct {
			Items []string
		}
		cfg := &Config{Items: []string{"$__secret{app:password}", "plain"}}
		if err := secrets.ExpandStruct(ctx, cfg, mgr); err != nil {
			t.Fatal(err)
		}
		if cfg.Items[0] != "s3cret" {
			t.Errorf("Items[0] = %q, want %q", cfg.Items[0], "s3cret")
		}
		if cfg.Items[1] != "plain" {
			t.Errorf("Items[1] = %q, want %q", cfg.Items[1], "plain")
		}
	})

	t.Run("map string values", func(t *testing.T) {
		type Config struct {
			Env map[string]string
		}
		cfg := &Config{Env: map[string]string{
			"PASS": "$__secret{app:password}",
			"HOST": "plain",
		}}
		if err := secrets.ExpandStruct(ctx, cfg, mgr); err != nil {
			t.Fatal(err)
		}
		if cfg.Env["PASS"] != "s3cret" {
			t.Errorf("Env[PASS] = %q, want %q", cfg.Env["PASS"], "s3cret")
		}
		if cfg.Env["HOST"] != "plain" {
			t.Errorf("Env[HOST] = %q, want %q", cfg.Env["HOST"], "plain")
		}
	})

	t.Run("nil manager", func(t *testing.T) {
		type Config struct{ V string }
		cfg := &Config{V: "$__secret{app:password}"}
		if err := secrets.ExpandStruct(ctx, cfg, nil); err != nil {
			t.Fatal(err)
		}
		if cfg.V != "$__secret{app:password}" {
			t.Error("nil manager should not modify struct")
		}
	})

	t.Run("not a pointer", func(t *testing.T) {
		type Config struct{ V string }
		cfg := Config{}
		err := secrets.ExpandStruct(ctx, cfg, mgr)
		if err == nil {
			t.Error("expected error for non-pointer")
		}
	})

	t.Run("nil input", func(t *testing.T) {
		if err := secrets.ExpandStruct(ctx, nil, mgr); err != nil {
			t.Errorf("nil input should not error, got %v", err)
		}
	})
}

func TestExpander_Expand_ManagerError(t *testing.T) {
	mgr := &mockManager{err: errors.New("connection failed")}
	expander := secrets.New(mgr)

	_, err := expander.Expand(t.Context(), "val=$__secret{app:key}")
	if err == nil {
		t.Fatal("Expand() expected error for manager failure, got nil")
	}
}

func TestExpander_Expand_FailOnErrorDisabled(t *testing.T) {
	mgr := newMockManager(map[string]string{
		"app:password": "s3cret",
	})
	expander := secrets.New(mgr, secrets.WithFailOnError(false))
	ctx := t.Context()

	t.Run("missing secret returns empty string", func(t *testing.T) {
		got, err := expander.Expand(ctx, "$__secret{app:missing}")
		if err != nil {
			t.Fatalf("Expand() error = %v", err)
		}
		if got != "" {
			t.Errorf("Expand() = %q, want empty string", got)
		}
	})

	t.Run("manager error returns empty string", func(t *testing.T) {
		errMgr := &mockManager{err: errors.New("connection failed")}
		failOpenExpander := secrets.New(errMgr, secrets.WithFailOnError(false))
		got, err := failOpenExpander.Expand(ctx, "val=$__secret{app:key}")
		if err != nil {
			t.Fatalf("Expand() error = %v", err)
		}
		if got != "val=" {
			t.Errorf("got %q, want %q", got, "val=")
		}
	})

	t.Run("existing secret still resolves", func(t *testing.T) {
		got, err := expander.Expand(ctx, "$__secret{app:password}")
		if err != nil {
			t.Fatalf("Expand() error = %v", err)
		}
		if got != "s3cret" {
			t.Errorf("Expand() = %q, want %q", got, "s3cret")
		}
	})
}

func TestExpander_Expand_FailClosed_MultipleSecrets(t *testing.T) {
	mgr := newMockManager(map[string]string{
		"app:password": "s3cret",
	})
	expander := secrets.New(mgr)

	// First secret resolves but second one doesn't — should fail on the first failure
	_, err := expander.Expand(t.Context(), "$__secret{app:password} $__secret{app:missing}")
	if err == nil {
		t.Fatal("Expand() expected error when any secret fails, got nil")
	}
}

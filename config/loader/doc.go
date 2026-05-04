// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package loader provides multi-source configuration loading with YAML/TOML backends and environment variable mapping.
// It supports nested structures with "__" delimiter, array/map notation, default values, validation, ${VAR} substitution,
// and secret expansion using the $__secret{namespace:key} syntax.
//
// All operations are thread-safe once the loader is initialized.
//
// # Features
//
//   - Multi-source loading: files, environment variables, default values.
//   - Multiple backends: YAML (yaml.v3), TOML.
//   - Variable substitution: ${VAR} or ${VAR:default}.
//   - Secret expansion: $__secret{namespace:key} for secure secret injection.
//   - Automatic validation: uses struct tags for validation logic.
//   - Nested configuration: supports complex structures and maps.
//
// # Environment variable expansion in values
//
// Values read from environment variables (and env-defaults applied via
// the "default" struct tag) go through a $VAR expansion pass:
//
//   - "$VAR"   - replaced with the value of VAR (empty if unset).
//   - "$$"     - literal "$" (escape; the second "$" is consumed).
//   - "$$VAR"  - literal "$VAR" (the second "$" is consumed; VAR is NOT expanded).
//   - bare "$" - preserved as a literal when not followed by [A-Za-z0-9_].
//
// The "${VAR}" form is NOT recognized by this pass; it is handled
// separately for default-value templates.
//
// # Secret Expansion
//
// The loader supports automatic expansion of secret placeholders using the syntax:
//
//	$__secret{namespace:key}
//
// Where namespace is a logical grouping and key is the specific secret identifier.
// Secrets are retrieved from a secrets.Manager which can be configured using WithSecretsManager.
//
// Secret expansion works in both configuration files and environment variables:
//
//	# config.yaml
//	database:
//	  password: $__secret{myapp:db_password}
//	  host: ${DB_HOST}
//
// If a secret is not found, it is replaced with an empty string.
//
// Example with secrets:
//
//	manager, _ := secrets.New[string](provider)
//	p := loader.New(nil,
//	    loader.WithPath("config.yaml"),
//	    loader.WithSecretsManager(manager),
//	)
//	cfg := &Config{}
//	p.Load(cfg)
//
// Example without secrets:
//
//	type Config struct {
//	    Database struct {
//	        Host string `yaml:"host" default:"localhost" env:"DB_HOST"`
//	        Port int    `yaml:"port" default:"5432" env:"DB_PORT"`
//	    } `yaml:"database"`
//	}
//	cfg := &Config{}
//	p := loader.New(&yaml3.Backend{}, loader.WithPath("config.yaml"))
//	p.Load(cfg)
package loader

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
// # Unknown keys
//
// A file key that binds to no field of the configuration struct, such as a
// misspelled "enable" for "enabled", fails the load with an [ErrDecode] error
// wrapping [ErrUnknownField] that names every unknown key of the file. A map
// accepts any entry key, but the struct values in it are checked; content
// under interface fields (including map[string]any) and types with their own
// unmarshaler is free-form. [WithAllowUnknownFields] restores ignoring unknown keys. The check
// needs a backend implementing backend.StrictDecoder, as both built-in
// backends do; other backends decode without it.
//
// # Environment variable names
//
// Every path of the environment loader (plain fields, indexed slices, nested
// sections and map entries) names variables by one rule:
//
//   - With a prefix ([WithEnvPrefix]), only variables carrying it are read;
//     any other variable is ignored, and the prefix is stripped before the
//     rest of the name is matched. The prefix is normalized by
//     appinfo.NormalizeEnvKeyPrefix: "app", "APP" and "APP_" all mean "APP_".
//   - A field holding a value (anything but a struct section) with an `env`
//     tag is named by the tag alone; its path name does not apply. A tag
//     starting with "APP_" or with the prefix maps to the prefix followed by
//     the rest of the tag. Entries of such a slice or map are
//     TAG__<index|key>, also when the tag contains the delimiter; the tag
//     claims a variable only when the rest of its name addresses an entry,
//     and the longest such tag wins. A tag of a scalar has no entries.
//   - Any other field is named by its path: one name per level joined by the
//     section delimiter ("__"). A level answers to its struct tag name and to
//     its Go field name, both in SCREAMING_SNAKE_CASE and matched exactly.
//     Embedded structs add no level. Slice indices and map keys are levels
//     taken verbatim; NAME[key] is read as NAME, the delimiter and key.
//   - Inside slice elements and map values `env` tags are ignored and the
//     path name applies.
//
// For a field MongoDB tagged yaml:"mongodb", both APP_MONGODB__DATABASE and
// APP_MONGO_DB__DATABASE set mongodb.database. When both are set, the struct
// tag spelling wins on every path: of the variables that set one value, only
// the one with the fewest levels spelled by Go field name over its whole name
// is applied, then the first in name order. HOSTS__0 and HOSTS__00 set one
// element, and a variable an env tag names whole belongs to that field only.
//
// "APP_" at the start of an env tag stands for the configured prefix: with
// prefix "RADAR", env:"APP_NAME" reads RADAR_NAME, and env:"RADAR_APP_NAME"
// reads RADAR_APP_NAME. env:"-" keeps a value field out of the environment.
// Errors name a variable and never quote its value, which may be a secret.
//
// A slice index must not be negative, and an index past the slice may grow it
// to at most [DefaultMaxEnvSliceLen] elements ([WithMaxEnvSliceLen]); any
// other index fails the load with [ErrEnvIndexOutOfRange]. Recursive types
// (Next *Node in Node) are supported: the environment reaches nested nodes by
// their path, allocating them on the way, to any depth its names spell out.
// Pointer cycles in the value passed to Load are followed once by defaults
// and secret expansion.
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

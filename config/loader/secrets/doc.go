// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package secrets provides secret expansion functionality for configuration loading.
// It enables automatic substitution of secret placeholders in configuration structs
// and environment variables using the $__secret{namespace:key} syntax.
//
// The package integrates with the security/secrets package to retrieve secret values
// from various storage backends (HashiCorp Vault, Google Cloud Secret Manager,
// Yandex Cloud Lockbox, or in-memory storage).
//
// # Syntax
//
// Secret placeholders use the following syntax:
//
//	$__secret{namespace:key}
//
// Where:
//   - namespace: A logical grouping for secrets (e.g., "myapp", "database", "api")
//   - key: The specific secret identifier within the namespace
//
// The namespace and key are combined into a single lookup key "namespace:key"
// that is passed to the secrets Manager.
//
// # Configuration File Example
//
//	database:
//	  host: ${DB_HOST}
//	  password: $__secret{myapp:db_password}
//	  api_key: $__secret{external:stripe_api_key}
//
// # Environment Variable Example
//
//	export API_KEY="$__secret{myapp:api_key}"
//	export DB_PASSWORD="prefix_$__secret{database:password}_suffix"
//
// # Reflection-Based Expansion
//
// The primary method for configuration files is ExpandStruct, which uses reflection
// to walk through all string fields of a struct and expand secret placeholders.
// This approach provides:
//   - Precise expansion only in actual string field values
//   - Better error messages with field path context
//   - Correct handling of YAML comments (they are not affected)
//   - Support for nested structs, pointers, slices, and maps
//
// Example:
//
//	type Config struct {
//	    Database struct {
//	        Password string `yaml:"password"`
//	    } `yaml:"database"`
//	}
//	cfg := &Config{}
//	// After loading config from YAML...
//	err := secrets.ExpandStruct(ctx, cfg, manager)
//
// # Error Handling
//
// When a secret is not found or an error occurs during retrieval:
//   - The placeholder is replaced with an empty string
//   - An error is logged (if a logger is configured)
//   - Processing continues with remaining placeholders
//
// Configuration loading therefore does not fail on missing secrets;
// the application decides how to handle the empty values.
//
// # Thread Safety
//
// The Expander is thread-safe and can be used concurrently from multiple goroutines.
// It relies on the underlying secrets.Manager which is also thread-safe.
//
// # Performance
//
// The Expander uses the secrets Manager's built-in caching mechanism. When
// force=true is used (the default), secrets are fetched from the cache first,
// with automatic fallback to the storage provider on cache misses.
package secrets

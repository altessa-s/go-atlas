// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"regexp"
	"strings"

	"github.com/altessa-s/go-atlas/security/secrets"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// secretPattern matches the $__secret{namespace:key} syntax.
// Capture groups:
//  1. namespace - the logical grouping for secrets
//  2. key - the specific secret identifier
var secretPattern = regexp.MustCompile(`\$__secret\{([^:}]+):([^}]+)\}`)

// Expander handles expansion of secret placeholders in strings.
// It uses a secrets.Manager to retrieve secret values and replaces
// placeholders with the actual secret data.
//
// Thread Safety:
// The Expander is thread-safe and can be used concurrently from multiple goroutines.
type Expander struct {
	manager     Manager
	logger      *slog.Logger
	failOnError bool
}

// Manager defines the interface for retrieving secrets.
// This interface is satisfied by *secrets.Manager[string].
type Manager interface {
	// Value retrieves a secret value by key.
	// When force is true, it will fetch from storage on cache miss.
	// When force is false, it returns ErrNotFound on cache miss.
	Value(ctx context.Context, key string, force bool) (*secrets.Value[string], error)
}

// New creates a new Expander with the specified secrets manager.
// If manager is nil, the Expander will replace all placeholders with empty strings.
//
// Parameters:
//   - manager: The secrets manager to use for retrieving secret values
//   - opts: Optional configuration options
//
// Returns a configured Expander instance.
func New(manager Manager, opts ...Option) *Expander {
	e := &Expander{
		manager:     manager,
		logger:      slog.Default(),
		failOnError: true,
	}

	for _, opt := range opts {
		opt(e)
	}

	if !e.failOnError {
		e.logger.Warn("secret expander configured in fail-open mode (WithFailOnError(false)): " +
			"unresolved secret placeholders will be replaced with empty strings instead of failing. " +
			"This can silently produce empty credentials and allow unauthenticated access — do not use in production")
	}

	return e
}

// Option is a functional option for configuring the Expander.
type Option func(*Expander)

// WithLogger sets the logger for the Expander.
// The logger is used to log errors when secret retrieval fails.
func WithLogger(logger *slog.Logger) Option {
	return func(e *Expander) {
		if logger != nil {
			e.logger = logger
		}
	}
}

// WithFailOnError controls whether secret expansion errors cause Expand to return
// an error (fail-closed) or silently replace with empty string (fail-open).
// Default is true (fail-closed) to prevent authorization bypass when secrets
// fail to resolve.
func WithFailOnError(fail bool) Option {
	return func(e *Expander) {
		e.failOnError = fail
	}
}

// Expand processes a string and replaces all $__secret{namespace:key} placeholders
// with actual secret values retrieved from the secrets manager.
//
// Parameters:
//   - ctx: Context for cancellation, deadlines, and tracing
//   - content: The string containing secret placeholders to expand
//
// Returns the string with all placeholders replaced. If a secret is not found
// or an error occurs, the placeholder is replaced with an empty string.
//
// Example:
//
//	expander := secrets.New(manager)
//	result, err := expander.Expand(ctx, "password: $__secret{myapp:db_password}")
//	// result: "password: actual_secret_value"
func (e *Expander) Expand(ctx context.Context, content string) (string, error) {
	if e.manager == nil {
		// No manager configured - replace all placeholders with empty strings
		return secretPattern.ReplaceAllString(content, ""), nil
	}

	// Find all matches first to avoid issues with replacement affecting indices
	matches := secretPattern.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content, nil
	}

	// Build result using strings.Builder for efficiency
	var result strings.Builder
	result.Grow(len(content))

	lastIndex := 0
	for _, match := range matches {
		// match[0:2] - full match
		// match[2:4] - namespace group
		// match[4:6] - key group
		fullMatchStart, fullMatchEnd := match[0], match[1]
		namespaceStart, namespaceEnd := match[2], match[3]
		keyStart, keyEnd := match[4], match[5]

		// Write content before this match
		result.WriteString(content[lastIndex:fullMatchStart])

		// Extract namespace and key
		namespace := content[namespaceStart:namespaceEnd]
		key := content[keyStart:keyEnd]

		// Build the full secret key as "namespace:key"
		secretKey := namespace + ":" + key

		// Retrieve the secret value
		secretValue, err := e.getSecretValue(ctx, secretKey)
		if err != nil {
			// Log the namespace/key (operator-private context) ONCE at
			// debug level. The returned error is intentionally redacted
			// — failOnError propagates this to operator-visible log
			// streams (CI, ops dashboards), and a literal secret name
			// like "vault_unseal_key" or "mongo_root_password" in those
			// streams is information leak even without the secret value.
			e.logger.DebugContext(ctx, "secret expansion target",
				slog.String("namespace", namespace),
				slog.String("key", key))
			if e.failOnError {
				return "", coreerrs.Wrap(err, "secret expansion failed; see debug log for the offending key")
			}
			e.logger.WarnContext(ctx, "failed to retrieve secret",
				slog.String("namespace", namespace),
				slog.String("key", key),
				slog.Any("error", err))
			secretValue = ""
		}

		// Write the secret value (or empty string)
		result.WriteString(secretValue)

		lastIndex = fullMatchEnd
	}

	// Write remaining content after last match
	result.WriteString(content[lastIndex:])

	return result.String(), nil
}

// getSecretValue retrieves a secret value from the manager.
// Returns the secret value or an error if retrieval fails.
func (e *Expander) getSecretValue(ctx context.Context, key string) (string, error) {
	value, err := e.manager.Value(ctx, key, true)
	if err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			return "", err
		}
		return "", err
	}

	return value.Value, nil
}

// HasSecrets checks if the content contains any secret placeholders.
// This can be used to optimize processing by skipping expansion
// when no placeholders are present.
//
// Parameters:
//   - content: The string to check for secret placeholders
//
// Returns true if the content contains at least one $__secret{...} placeholder.
func HasSecrets(content string) bool {
	return secretPattern.MatchString(content)
}

// ExpandString is a convenience function that expands secrets in a string
// using the provided manager. It creates a temporary Expander instance.
//
// Parameters:
//   - ctx: Context for cancellation, deadlines, and tracing
//   - content: The string containing secret placeholders to expand
//   - manager: The secrets manager to use for retrieving secret values
//
// Returns the string with all placeholders replaced.
func ExpandString(ctx context.Context, content string, manager Manager) (string, error) {
	if manager == nil || !HasSecrets(content) {
		return content, nil
	}

	expander := New(manager)
	return expander.Expand(ctx, content)
}

// ExpandEnvValue expands secret placeholders in an environment variable value.
// This function is designed to be called during environment variable processing.
//
// Parameters:
//   - ctx: Context for cancellation, deadlines, and tracing
//   - value: The environment variable value to process
//   - manager: The secrets manager to use for retrieving secret values
//
// Returns the value with all secret placeholders replaced, or an error if
// secret resolution fails (fail-closed by default).
func ExpandEnvValue(ctx context.Context, value string, manager Manager) (string, error) {
	if manager == nil || !HasSecrets(value) {
		return value, nil
	}

	expander := New(manager)
	return expander.Expand(ctx, value)
}

// ExpandStruct walks through a struct using reflection and expands all
// $__secret{namespace:key} placeholders found in string fields.
//
// The function handles:
//   - string fields
//   - *string fields (pointer to string)
//   - []string fields (slice of strings)
//   - map[K]string fields (maps with string values)
//   - Nested structs and pointers to structs
//   - Slices and maps containing structs
//
// Parameters:
//   - ctx: Context for cancellation, deadlines, and tracing
//   - v: Pointer to the struct to process
//   - manager: The secrets manager to use for retrieving secret values
//
// Returns an error if the input is not a pointer to a struct.
//
// Example:
//
//	type Config struct {
//	    Database struct {
//	        Password string `yaml:"password"`
//	    } `yaml:"database"`
//	}
//	cfg := &Config{Database: struct{Password string}{Password: "$__secret{app:db_pass}"}}
//	err := secrets.ExpandStruct(ctx, cfg, manager)
//	// cfg.Database.Password is now the actual secret value
func ExpandStruct(ctx context.Context, v any, manager Manager) error {
	if manager == nil {
		return nil
	}

	expander := New(manager)
	return expander.ExpandStruct(ctx, v)
}

// ExpandStruct walks through a struct using reflection and expands all
// $__secret{namespace:key} placeholders found in string fields.
//
// Parameters:
//   - ctx: Context for cancellation, deadlines, and tracing
//   - v: Pointer to the struct to process
//
// Returns an error if the input is not a pointer to a struct.
func (e *Expander) ExpandStruct(ctx context.Context, v any) error {
	if e.manager == nil {
		return nil
	}

	if v == nil {
		return nil
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr {
		return fmt.Errorf("ExpandStruct: expected pointer, got %s", rv.Kind())
	}

	if rv.IsNil() {
		return nil
	}

	return e.walkValue(ctx, rv.Elem(), "")
}

// walkValue recursively walks through a reflect.Value and expands secrets.
func (e *Expander) walkValue(ctx context.Context, v reflect.Value, path string) error {
	if !v.IsValid() {
		return nil
	}

	switch v.Kind() {
	case reflect.String:
		return e.expandStringField(ctx, v, path)

	case reflect.Ptr:
		if v.IsNil() {
			return nil
		}
		return e.walkValue(ctx, v.Elem(), path)

	case reflect.Struct:
		return e.walkStruct(ctx, v, path)

	case reflect.Slice:
		return e.walkSlice(ctx, v, path)

	case reflect.Map:
		return e.walkMap(ctx, v, path)

	case reflect.Interface:
		if v.IsNil() {
			return nil
		}
		// For interface values, we need to get the underlying element
		elem := v.Elem()
		if elem.Kind() == reflect.Ptr && !elem.IsNil() {
			return e.walkValue(ctx, elem.Elem(), path)
		}
		return e.walkValue(ctx, elem, path)

	default:
		// Skip other types (numbers, bools, channels, funcs, etc.)
		return nil
	}
}

// walkStruct walks through all fields of a struct.
func (e *Expander) walkStruct(ctx context.Context, v reflect.Value, path string) error {
	t := v.Type()

	for i := range v.NumField() {
		field := v.Field(i)
		fieldType := t.Field(i)

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		fieldPath := fieldType.Name
		if path != "" {
			fieldPath = path + "." + fieldType.Name
		}

		if err := e.walkValue(ctx, field, fieldPath); err != nil {
			return err
		}
	}

	return nil
}

// walkSlice walks through all elements of a slice.
func (e *Expander) walkSlice(ctx context.Context, v reflect.Value, path string) error {
	for i := range v.Len() {
		elem := v.Index(i)
		elemPath := fmt.Sprintf("%s[%d]", path, i)

		if err := e.walkValue(ctx, elem, elemPath); err != nil {
			return err
		}
	}

	return nil
}

// walkMap walks through all values of a map.
func (e *Expander) walkMap(ctx context.Context, v reflect.Value, path string) error {
	if v.IsNil() {
		return nil
	}

	iter := v.MapRange()
	for iter.Next() {
		key := iter.Key()
		val := iter.Value()

		keyStr := fmt.Sprintf("%v", key.Interface())
		elemPath := fmt.Sprintf("%s[%s]", path, keyStr)

		// For maps, we need to handle the value specially since map values are not addressable
		if val.Kind() == reflect.String {
			str := val.String()
			if HasSecrets(str) {
				expanded, err := e.Expand(ctx, str)
				if err != nil {
					return coreerrs.WrapOperation(err, "expand secret at "+elemPath)
				}
				v.SetMapIndex(key, reflect.ValueOf(expanded))
			}
		} else if val.Kind() == reflect.Ptr || val.Kind() == reflect.Struct ||
			val.Kind() == reflect.Slice || val.Kind() == reflect.Map {
			// For complex types in maps, we need to make a copy, modify it, and set it back
			if err := e.walkMapValue(ctx, v, key, val, elemPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// walkMapValue handles complex values in maps by creating addressable copies.
func (e *Expander) walkMapValue(ctx context.Context, mapVal reflect.Value, key, val reflect.Value, path string) error {
	// For pointer types, we can walk directly if not nil
	if val.Kind() == reflect.Ptr && !val.IsNil() {
		return e.walkValue(ctx, val.Elem(), path)
	}

	// For struct types in maps, we need to copy, modify, and set back
	if val.Kind() == reflect.Struct {
		// Create a new pointer to a copy of the struct
		newVal := reflect.New(val.Type())
		newVal.Elem().Set(val)

		if err := e.walkValue(ctx, newVal.Elem(), path); err != nil {
			return err
		}

		mapVal.SetMapIndex(key, newVal.Elem())
		return nil
	}

	// For slices and nested maps, handle recursively
	if val.Kind() == reflect.Slice || val.Kind() == reflect.Map {
		// Create a copy and process it
		newVal := reflect.New(val.Type())
		newVal.Elem().Set(val)

		if err := e.walkValue(ctx, newVal.Elem(), path); err != nil {
			return err
		}

		mapVal.SetMapIndex(key, newVal.Elem())
	}

	return nil
}

// expandStringField expands secrets in a string field if it contains placeholders.
func (e *Expander) expandStringField(ctx context.Context, v reflect.Value, path string) error {
	if !v.CanSet() {
		return nil
	}

	str := v.String()
	if !HasSecrets(str) {
		return nil
	}

	expanded, err := e.Expand(ctx, str)
	if err != nil {
		return coreerrs.WrapOperation(err, "expand secret at "+path)
	}

	v.SetString(expanded)
	return nil
}

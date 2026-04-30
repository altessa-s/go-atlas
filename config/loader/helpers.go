// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/altessa-s/go-atlas/core/runtime/appinfo"

	loadersecrets "github.com/altessa-s/go-atlas/config/loader/secrets"
	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

const (
	// Environment variable unwrapping constants
	maxDepth           = 10
	envVarDollarPrefix = "$"

	// dollarSentinel temporarily replaces escaped `$$` sequences during
	// expansion so the resulting literal `$` is not re-interpreted as an
	// env-var prefix by subsequent scan iterations or recursive calls.
	// The control characters at both ends ensure it cannot collide with any
	// realistic env-var value or user-supplied string.
	dollarSentinel = "\x00ATL_LITERAL_DOLLAR\x00"
)

// ErrInvalidConfig is returned when the config parameter is invalid.
var ErrInvalidConfig = errors.New("config must be a struct pointer")

// envsWithSecrets returns a map of environment variables with optional secrets expansion.
// It parses os.Environ(), unwraps any environment variable references, and expands
// $__secret{namespace:key} placeholders if a secrets manager is provided.
//
// Parameters:
//   - ctx: The context for secrets operations (can be nil, defaults to context.Background())
//   - manager: The secrets manager for expanding secret placeholders (can be nil)
//   - strict: When true, returns an error if any referenced environment variable is undefined
//
//nolint:contextcheck // context is properly handled within function
func envsWithSecrets(ctx context.Context, manager loadersecrets.Manager, strict bool) (map[string]string, error) {
	ctx = corecontext.OrBackground(ctx)

	envs := make(map[string]string)

	for _, s := range os.Environ() {
		for j := range len(s) {
			if s[j] == '=' {
				key, value := s[:j], s[j+1:]
				normalizedKey := normalizeEnvKey(key)

				var unwrappedValue string
				if strict {
					var err error
					unwrappedValue, err = unwrapEnvValueStrict(value)
					if err != nil {
						return nil, err
					}
				} else {
					unwrappedValue = unwrapEnvValue(value)
				}

				// Then expand secrets if manager is provided
				if manager != nil {
					expanded, err := loadersecrets.ExpandEnvValue(ctx, unwrappedValue, manager)
					if err != nil {
						return nil, coreerrs.WrapOperation(err, "expand secrets in environment variable "+key)
					}
					unwrappedValue = expanded
				}

				envs[normalizedKey] = unwrappedValue

				break
			}
		}
	}

	return envs, nil
}

// normalizeEnvKey converts square bracket syntax to double underscore syntax.
// Examples:
//   - "NATS__CONSUMERS[keycloak_sync]__DESCRIPTION" → "NATS__CONSUMERS__keycloak_sync__DESCRIPTION"
//   - "BACK_OFF[0]" → "BACK_OFF__0"
//   - "NORMAL_KEY" → "NORMAL_KEY" (unchanged)
func normalizeEnvKey(key string) string {
	// Quick check if normalization is needed
	if !strings.Contains(key, "[") {
		return key
	}

	// Replace [ with __ and remove ]
	normalized := strings.ReplaceAll(key, "[", "__")
	normalized = strings.ReplaceAll(normalized, "]", "")
	return normalized
}

// unwrapEnvValue unwraps the environment variable value.
// Prevents infinite recursion by limiting depth and tracking visited variables.
//
// Supported syntax in values:
//   - "$VAR"          — expanded to the value of VAR (or empty if undefined).
//   - "$$"            — literal "$" (escape; the second "$" is consumed).
//   - "$$VAR"         — literal "$VAR" (no expansion of VAR).
//   - bare "$" with no following [A-Za-z0-9_] — preserved as a literal "$".
//
// The "${VAR}" form is not handled here; it is processed separately by
// substituteEnvVariables when applied to default values and templates.
func unwrapEnvValue(value string) string {
	return decodeDollarSentinel(unwrapEnvValueWithDepthAndOriginal(value, value, 0, make(map[string]bool)))
}

// decodeDollarSentinel converts the dollar sentinel back to a literal `$`.
// Called by the public entry points after expansion completes — never inside
// the recursive workhorse, so escaped `$` survives every recursive pass.
func decodeDollarSentinel(s string) string {
	if !strings.Contains(s, dollarSentinel) {
		return s
	}
	return strings.ReplaceAll(s, dollarSentinel, envVarDollarPrefix)
}

func unwrapEnvValueWithDepthAndOriginal(value, original string, depth int, visited map[string]bool) string {
	if depth >= maxDepth {
		return original // Stop at max depth, return original value
	}

	// Handle simple case where entire value is a single environment variable
	if strings.HasPrefix(value, envVarDollarPrefix) && !strings.ContainsAny(value[1:], "$/\\") {
		key := value[1:]

		if visited[key] {
			// Circular reference detected - return current value if env var not set
			envValue := os.Getenv(key)
			if envValue == "" {
				return value
			}
			// Return the environment value (next in chain) if it's a simple variable
			if strings.HasPrefix(envValue, envVarDollarPrefix) && !strings.ContainsAny(envValue[1:], "$/\\") {
				return envValue
			}
			return value // Return current value for complex expressions
		}

		envValue := os.Getenv(key)
		if envValue == "" {
			return "" // Return empty string for undefined variables
		}

		// Mark this key as visited before recursing
		visited[key] = true
		result := unwrapEnvValueWithDepthAndOriginal(envValue, original, depth+1, visited)
		// Don't unmark - leave it visited to detect future circular references

		return result
	}

	// Handle mixed strings with embedded environment variables.
	// Track scan offset so a literal `$` (no variable name following, or a name
	// already visited) is preserved in place — important for values like
	// regex patterns ending in `$` or arbitrary text containing a bare `$`.
	result := value
	changed := false
	offset := 0

	for {
		dollarIndex := strings.Index(result[offset:], envVarDollarPrefix)
		if dollarIndex == -1 {
			break
		}
		dollarIndex += offset

		// Escape: `$$` collapses to a literal `$` (encoded as a sentinel so it
		// is not re-interpreted on subsequent passes). The second `$` is
		// consumed so a following name is NOT expanded (e.g. `$$VAR` → `$VAR`).
		if dollarIndex+1 < len(result) && result[dollarIndex+1] == '$' {
			result = result[:dollarIndex] + dollarSentinel + result[dollarIndex+2:]
			offset = dollarIndex + len(dollarSentinel)
			changed = true
			continue
		}

		// Find the end of the variable name (next non-alphanumeric character or end of string)
		start := dollarIndex + 1
		end := start
		for end < len(result) && (result[end] == '_' ||
			(result[end] >= 'A' && result[end] <= 'Z') ||
			(result[end] >= 'a' && result[end] <= 'z') ||
			(result[end] >= '0' && result[end] <= '9')) {
			end++
		}

		if end > start {
			key := result[start:end]
			if !visited[key] {
				envValue := os.Getenv(key)
				if envValue != "" {
					// Replace $VAR with its value and continue scanning after the substitution.
					result = result[:dollarIndex] + envValue + result[end:]
					offset = dollarIndex + len(envValue)
					changed = true
					continue
				}
				// Remove undefined $VAR entirely; resume scanning from the same position.
				result = result[:dollarIndex] + result[end:]
				offset = dollarIndex
				changed = true
				continue
			}
		}

		// No variable name follows this `$` (or the name is already visited):
		// preserve it as a literal and advance past it.
		offset = dollarIndex + 1
	}

	// Recursively process the result if it changed and we haven't hit max depth
	if changed && depth < maxDepth-1 {
		return unwrapEnvValueWithDepthAndOriginal(result, original, depth+1, visited)
	}

	return result
}

// userHomeDir returns the home directory of the user.
// It handles both Windows and Unix-like systems.
func userHomeDir() string {
	return appinfo.HomeDir()
}

// assertStructPointer validates that the provided configuration is a pointer to a struct.
// This is required for the configuration loading to work properly.
func assertStructPointer(conf any) error {
	t := reflect.TypeOf(conf)
	if t.Kind() != reflect.Pointer || indirectType(t).Kind() != reflect.Struct {
		return ErrInvalidConfig
	}
	return nil
}

// substituteEnvVariables performs environment variable substitution in a string.
// It replaces all occurrences of ${VAR_NAME} with the corresponding environment variable values.
// This function handles multiple variables within the same string and is used for text-level substitution.
func substituteEnvVariables(input string) string {
	return os.Expand(input, os.Getenv)
}

// unwrapEnvValueStrict is the strict variant of unwrapEnvValue.
// It returns an error if any referenced environment variable is not defined.
// A variable that is defined but set to an empty string is not considered an error.
//
// Supported syntax matches unwrapEnvValue: "$VAR" expands; "$$" yields a
// literal "$" (and the next char is not interpreted as a variable name);
// a bare "$" with no following [A-Za-z0-9_] is preserved as a literal.
func unwrapEnvValueStrict(value string) (string, error) {
	result, err := unwrapEnvValueStrictWithDepth(value, value, 0, make(map[string]bool))
	if err != nil {
		return "", err
	}
	return decodeDollarSentinel(result), nil
}

func unwrapEnvValueStrictWithDepth(value, original string, depth int, visited map[string]bool) (string, error) {
	if depth >= maxDepth {
		return original, nil
	}

	// Handle simple case where entire value is a single environment variable
	if strings.HasPrefix(value, envVarDollarPrefix) && !strings.ContainsAny(value[1:], "$/\\") {
		key := value[1:]

		if visited[key] {
			envValue, ok := os.LookupEnv(key)
			if !ok {
				return "", fmt.Errorf("%w: $%s", ErrUndefinedEnvVar, key)
			}
			if envValue == "" {
				return value, nil
			}
			if strings.HasPrefix(envValue, envVarDollarPrefix) && !strings.ContainsAny(envValue[1:], "$/\\") {
				return envValue, nil
			}
			return value, nil
		}

		envValue, ok := os.LookupEnv(key)
		if !ok {
			return "", fmt.Errorf("%w: $%s", ErrUndefinedEnvVar, key)
		}
		if envValue == "" {
			return "", nil
		}

		visited[key] = true
		return unwrapEnvValueStrictWithDepth(envValue, original, depth+1, visited)
	}

	// Handle mixed strings with embedded environment variables.
	// See unwrapEnvValueWithDepthAndOriginal — same logic.
	result := value
	changed := false
	offset := 0

	for {
		dollarIndex := strings.Index(result[offset:], envVarDollarPrefix)
		if dollarIndex == -1 {
			break
		}
		dollarIndex += offset

		// Escape: `$$` → literal `$` (encoded as a sentinel so it is not
		// re-interpreted on subsequent passes). Consume both characters so
		// a following name is not expanded (e.g. `$$VAR` → `$VAR`).
		if dollarIndex+1 < len(result) && result[dollarIndex+1] == '$' {
			result = result[:dollarIndex] + dollarSentinel + result[dollarIndex+2:]
			offset = dollarIndex + len(dollarSentinel)
			changed = true
			continue
		}

		start := dollarIndex + 1
		end := start
		for end < len(result) && (result[end] == '_' ||
			(result[end] >= 'A' && result[end] <= 'Z') ||
			(result[end] >= 'a' && result[end] <= 'z') ||
			(result[end] >= '0' && result[end] <= '9')) {
			end++
		}

		if end > start {
			key := result[start:end]
			if !visited[key] {
				envValue, ok := os.LookupEnv(key)
				if !ok {
					return "", fmt.Errorf("%w: $%s", ErrUndefinedEnvVar, key)
				}
				result = result[:dollarIndex] + envValue + result[end:]
				offset = dollarIndex + len(envValue)
				changed = true
				continue
			}
		}

		// No variable name follows this `$` (or the name is already visited):
		// preserve it as a literal and advance past it.
		offset = dollarIndex + 1
	}

	if changed && depth < maxDepth-1 {
		return unwrapEnvValueStrictWithDepth(result, original, depth+1, visited)
	}

	return result, nil
}

// substituteEnvVariablesStrict performs environment variable substitution in a string.
// Unlike substituteEnvVariables, it returns an error if any referenced variable is undefined.
// A variable that is defined but set to an empty string is not considered an error.
func substituteEnvVariablesStrict(input string) (string, error) {
	var firstErr error
	result := os.Expand(input, func(key string) string {
		val, ok := os.LookupEnv(key)
		if !ok && firstErr == nil {
			firstErr = fmt.Errorf("%w: ${%s}", ErrUndefinedEnvVar, key)
		}
		return val
	})
	if firstErr != nil {
		return "", firstErr
	}
	return result, nil
}

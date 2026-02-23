// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import (
	"context"
	"errors"
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
//
//nolint:contextcheck // context is properly handled within function
func envsWithSecrets(ctx context.Context, manager loadersecrets.Manager) (map[string]string, error) {
	ctx = corecontext.OrBackground(ctx)

	envs := make(map[string]string)

	for _, s := range os.Environ() {
		for j := range len(s) {
			if s[j] == '=' {
				key, value := s[:j], s[j+1:]
				normalizedKey := normalizeEnvKey(key)

				// First unwrap regular environment variables
				unwrappedValue := unwrapEnvValue(value)

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
func unwrapEnvValue(value string) string {
	return unwrapEnvValueWithDepthAndOriginal(value, value, 0, make(map[string]bool))
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

	// Handle mixed strings with embedded environment variables
	result := value
	changed := false

	for {
		// Find the first $VAR pattern
		dollarIndex := strings.Index(result, envVarDollarPrefix)
		if dollarIndex == -1 {
			break // No more $ found
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
					// Replace $VAR with its value
					result = result[:dollarIndex] + envValue + result[end:]
					changed = true
					continue // Continue processing from the beginning
				} else {
					// Remove undefined $VAR entirely
					result = result[:dollarIndex] + result[end:]
					changed = true
					continue
				}
			}
		}

		// Move past this $ if we couldn't replace it
		result = result[:dollarIndex] + result[dollarIndex+1:]
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
// substituteEnvVariables performs environment variable substitution in a string.
// It replaces all occurrences of ${VAR_NAME} with the corresponding environment variable values.
// This function handles multiple variables within the same string and is used for text-level substitution.
func substituteEnvVariables(input string) string {
	return os.Expand(input, os.Getenv)
}

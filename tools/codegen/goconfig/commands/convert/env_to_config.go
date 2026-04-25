// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package convert

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

const (
	// envKeyValueParts is the expected number of parts when splitting KEY=VALUE
	envKeyValueParts = 2
)

// EnvToConfigConverter reconstructs a nested YAML or TOML configuration tree
// from a flat .env file. Keys are split on __ to recover the nesting structure,
// and SCREAMING_SNAKE_CASE segments are converted back to camelCase. Values
// are automatically parsed into Go types (bool, int64, float64, []any, or string).
type EnvToConfigConverter struct {
	parseComments bool
}

// NewEnvToConfigConverter creates a new .env to config converter.
func NewEnvToConfigConverter() *EnvToConfigConverter {
	return &EnvToConfigConverter{}
}

// SetParseComments enables or disables parsing of commented lines.
func (conv *EnvToConfigConverter) SetParseComments(parse bool) {
	conv.parseComments = parse
}

// Convert parses the .env file at fromPath line-by-line and writes the
// reconstructed config tree to toPath in the given format ("yaml" or "toml").
// Lines that fail KEY=VALUE parsing cause an immediate error with a line number.
// When parseComments is enabled, lines starting with # are included rather
// than skipped.
// #nosec G304 -- CLI tool, path from command-line arguments
func (conv *EnvToConfigConverter) Convert(fromPath, toPath, format string) error {
	// Read .env file
	file, err := os.Open(fromPath)
	if err != nil {
		return coreerrs.Wrapf(err, "failed to read .env file %s", fromPath)
	}
	defer func() { _ = file.Close() }()

	// Parse .env file
	envVars := make(map[string]string)
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			continue
		}

		// Handle commented lines
		if comment, ok := strings.CutPrefix(line, "#"); ok {
			if !conv.parseComments {
				continue
			}
			line = strings.TrimSpace(comment)
			if line == "" {
				continue
			}
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", envKeyValueParts)
		if len(parts) != envKeyValueParts {
			return fmt.Errorf("invalid line %d: %s", lineNum, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes from value if present
		value = strings.Trim(value, `"'`)

		envVars[key] = value
	}

	if scanErr := scanner.Err(); scanErr != nil {
		return coreerrs.WrapOperation(scanErr, "scan .env file")
	}

	// Build config structure
	configData := conv.buildConfigStructure(envVars)

	return writeConfigFile(toPath, format, configData)
}

// buildConfigStructure builds nested config structure from flat env variables.
func (conv *EnvToConfigConverter) buildConfigStructure(envVars map[string]string) map[string]any {
	result := make(map[string]any)

	for key, value := range envVars {
		conv.setNestedValue(result, key, value)
	}

	return result
}

// setNestedValue sets a value in nested map structure based on key path.
func (conv *EnvToConfigConverter) setNestedValue(data map[string]any, key, value string) {
	// Split by __ (section delimiter) and . (array index)
	parts := conv.splitKey(key)
	if len(parts) == 0 {
		return
	}

	current := data
	for i, part := range parts[:len(parts)-1] {
		// Convert SCREAMING_SNAKE_CASE to camelCase
		fieldName := corestrings.ScreamingSnakeToCamelCase(part)

		// Check if next part is array index
		if i+1 < len(parts) {
			nextPart := parts[i+1]
			if idx, parseErr := strconv.Atoi(nextPart); parseErr == nil {
				// Next is array index
				if _, exists := current[fieldName]; !exists {
					current[fieldName] = make([]any, 0)
				}

				// Ensure it's a slice
				slice, ok := current[fieldName].([]any)
				if !ok {
					slice = make([]any, 0)
					current[fieldName] = slice
				}

				// Extend slice if needed
				for len(slice) <= idx {
					slice = append(slice, make(map[string]any))
				}
				current[fieldName] = slice

				// Move to the array element
				if element, ok := slice[idx].(map[string]any); ok {
					current = element
				} else {
					newMap := make(map[string]any)
					slice[idx] = newMap
					current[fieldName] = slice
					current = newMap
				}

				continue
			}
		}

		// Regular nested field
		if _, exists := current[fieldName]; !exists {
			current[fieldName] = make(map[string]any)
		}

		// Move to nested map
		if nestedMap, ok := current[fieldName].(map[string]any); ok {
			current = nestedMap
		} else {
			// Create new map if current value is not a map
			newMap := make(map[string]any)
			current[fieldName] = newMap
			current = newMap
		}
	}

	// Set the final value
	lastPart := parts[len(parts)-1]

	// Check if it's an array index
	if idx, err := strconv.Atoi(lastPart); err == nil {
		// This shouldn't happen in normal cases, but handle it
		_ = idx
		return
	}

	fieldName := corestrings.ScreamingSnakeToCamelCase(lastPart)
	current[fieldName] = conv.parseValue(value)
}

// splitKey splits environment variable key into parts.
// Handles both __ (section delimiter) and . (array index).
func (conv *EnvToConfigConverter) splitKey(key string) []string {
	// First split by __
	sections := strings.Split(key, "__")

	// Then split each section by . (for array indices)
	var parts []string
	for _, section := range sections {
		if strings.Contains(section, ".") {
			dotParts := strings.Split(section, ".")
			parts = append(parts, dotParts...)
		} else {
			parts = append(parts, section)
		}
	}

	return parts
}

// SCREAMING_SNAKE_CASE → camelCase conversion is implemented in core/text/strings.

// parseValue attempts to parse string value to appropriate type.
func (conv *EnvToConfigConverter) parseValue(value string) any {
	// Try array format [element1,element2,element3]
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		return conv.parseArray(value)
	}

	// Try boolean
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}

	// Try integer
	if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
		return intVal
	}

	// Try float
	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal
	}

	// Return as string
	return value
}

// parseArray parses array format [element1,element2,element3] into a slice.
func (conv *EnvToConfigConverter) parseArray(value string) any {
	// Remove brackets
	content := value[1 : len(value)-1]

	// Handle empty array
	if content == "" {
		return []any{}
	}

	// Split by comma
	elements := strings.Split(content, ",")
	result := make([]any, len(elements))

	for i, element := range elements {
		// Parse each element (but not as array to avoid recursion)
		element = strings.TrimSpace(element)

		// Try boolean
		if element == "true" {
			result[i] = true
			continue
		}
		if element == "false" {
			result[i] = false
			continue
		}

		// Try integer
		if intVal, err := strconv.ParseInt(element, 10, 64); err == nil {
			result[i] = intVal
			continue
		}

		// Try float
		if floatVal, err := strconv.ParseFloat(element, 64); err == nil {
			result[i] = floatVal
			continue
		}

		// Keep as string
		result[i] = element
	}

	return result
}

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
	configData, err := conv.buildConfigStructure(envVars)
	if err != nil {
		return err
	}

	return writeConfigFile(toPath, format, configData)
}

// buildConfigStructure builds nested config structure from flat env variables.
func (conv *EnvToConfigConverter) buildConfigStructure(envVars map[string]string) (map[string]any, error) {
	result := make(map[string]any)

	for key, value := range envVars {
		if err := conv.setNestedValue(result, key, value); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// setNestedValue sets a value in nested map structure based on key path.
// A purely numeric part following a field is an index into that field's
// array: SERVERS__0__HOST sets servers[0].host, and PORTS__0 sets ports[0].
func (conv *EnvToConfigConverter) setNestedValue(data map[string]any, key, value string) error {
	// Split by __ (section delimiter) and . (array index)
	parts := conv.splitKey(key)
	if len(parts) == 0 {
		return nil
	}

	current := data
	for i := 0; i < len(parts); {
		// Convert SCREAMING_SNAKE_CASE to camelCase
		fieldName := corestrings.ScreamingSnakeToCamelCase(parts[i])

		if i == len(parts)-1 {
			current[fieldName] = conv.parseValue(value)
			return nil
		}

		idx, parseErr := strconv.Atoi(parts[i+1])
		if parseErr != nil {
			// Regular nested field: move to (or create) the nested map.
			nestedMap, ok := current[fieldName].(map[string]any)
			if !ok {
				nestedMap = make(map[string]any)
				current[fieldName] = nestedMap
			}
			current = nestedMap
			i++
			continue
		}

		if idx < 0 {
			return fmt.Errorf("invalid array index %q in key %q", parts[i+1], key)
		}

		// The next part is an array index: extend the slice to cover it.
		slice, _ := current[fieldName].([]any)
		for len(slice) <= idx {
			slice = append(slice, make(map[string]any))
		}
		current[fieldName] = slice

		// A trailing index sets the array element itself.
		if i+1 == len(parts)-1 {
			slice[idx] = conv.parseValue(value)
			return nil
		}

		// Move to the array element, consuming both the field and the index.
		element, ok := slice[idx].(map[string]any)
		if !ok {
			element = make(map[string]any)
			slice[idx] = element
		}
		current = element
		i += 2
	}

	return nil
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

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package convert

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// ConfigToMarkdownConverter generates a Markdown reference document from a
// YAML or TOML configuration file. The output contains a table of contents
// and per-section tables listing each environment variable, its inferred Go
// type, and its default value. When a Claude API key is configured via
// [ConfigToMarkdownConverter.SetClaudeAPI], an additional "Description"
// column is populated by [ClaudeClient].
type ConfigToMarkdownConverter struct {
	claudeAPIKey string
}

// NewConfigToMarkdownConverter creates a new config to Markdown converter.
func NewConfigToMarkdownConverter() *ConfigToMarkdownConverter {
	return &ConfigToMarkdownConverter{}
}

// SetClaudeAPI enables Claude API integration for generating descriptions.
func (conv *ConfigToMarkdownConverter) SetClaudeAPI(apiKey string) {
	conv.claudeAPIKey = apiKey
}

// Convert reads a YAML or TOML file at fromPath and writes a Markdown
// environment-variable reference to toPath. YAML input is always
// uncommented before parsing so template files produce complete output.
// #nosec G304 -- CLI tool, path from command-line arguments
func (conv *ConfigToMarkdownConverter) Convert(fromPath, toPath, format string) error {
	// Read file
	data, err := os.ReadFile(fromPath)
	if err != nil {
		return coreerrs.Wrapf(err, "failed to read config file %s", fromPath)
	}

	// Parse based on format
	var configData map[string]any

	switch format {
	case formatYAML:
		// Always uncomment YAML for markdown documentation
		converter := NewConfigToEnvConverter()
		uncommentedData := converter.UncommentYAML(data)
		if err := yaml.Unmarshal(uncommentedData, &configData); err != nil {
			return coreerrs.WrapOperation(err, "parse YAML")
		}
	case formatTOML:
		if err := toml.Unmarshal(data, &configData); err != nil {
			return coreerrs.WrapOperation(err, "parse TOML")
		}
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}

	return conv.ConvertData(configData, toPath)
}

// ConvertData generates a Markdown reference from pre-loaded configData and
// writes it to toPath. If a Claude API key has been set, descriptions are
// generated per-section via a single API call each; an API error for any
// section causes the entire conversion to fail.
func (conv *ConfigToMarkdownConverter) ConvertData(configData map[string]any, toPath string) error {
	// Flatten to env vars with type information
	envVars := conv.flattenConfigWithTypes(configData, "")

	// Sort keys for consistent output
	keys := make([]string, 0, len(envVars))
	for key := range envVars {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	// Group keys by top-level section
	sections := conv.groupBySection(keys)

	// Generate Markdown documentation
	var builder strings.Builder

	// Add header
	builder.WriteString("# Environment Variables Reference\n\n")
	builder.WriteString("This document describes all available environment variables for configuration.\n\n")

	// Add table of contents
	builder.WriteString("## Table of Contents\n\n")
	for _, sectionName := range getSortedSectionNames(sections) {
		anchor := strings.ToLower(strings.ReplaceAll(sectionName, "_", "-"))
		fmt.Fprintf(&builder, "- [%s Configuration](#%s-configuration)\n", formatSectionName(sectionName), anchor)
	}
	builder.WriteString("\n")

	// Generate descriptions with Claude if API key is provided
	var allDescriptions map[string]map[string]string
	if conv.claudeAPIKey != "" {
		allDescriptions = make(map[string]map[string]string)
		client := NewClaudeClient(conv.claudeAPIKey)

		sectionNames := getSortedSectionNames(sections)
		for _, sectionName := range sectionNames {
			sectionKeys := sections[sectionName]
			sectionVars := make(map[string]varInfo)
			for _, key := range sectionKeys {
				sectionVars[key] = envVars[key]
			}

			descriptions, err := client.GenerateDescriptions(sectionVars, sectionName)
			if err != nil {
				return coreerrs.Wrapf(err, "failed to generate descriptions for %s", sectionName)
			}
			allDescriptions[sectionName] = descriptions
		}
	}

	// Add sections with tables
	for _, sectionName := range getSortedSectionNames(sections) {
		sectionKeys := sections[sectionName]

		// Add section header
		fmt.Fprintf(&builder, "## %s Configuration\n\n", formatSectionName(sectionName))

		// Add table header
		if conv.claudeAPIKey != "" {
			builder.WriteString("| Environment Variable | Type | Default Value | Description |\n")
			builder.WriteString("|---------------------|------|---------------|-------------|\n")
		} else {
			builder.WriteString("| Environment Variable | Type | Default Value |\n")
			builder.WriteString("|---------------------|------|---------------|\n")
		}

		// Add variables in this section
		for _, key := range sectionKeys {
			vi := envVars[key]
			// Format value - use empty cell for empty strings, otherwise wrap in backticks
			valueCell := vi.Value
			if valueCell == "" {
				valueCell = " "
			} else {
				valueCell = fmt.Sprintf("`%s`", valueCell)
			}

			if conv.claudeAPIKey != "" {
				description := allDescriptions[sectionName][key]
				if description == "" {
					description = " "
				}
				fmt.Fprintf(&builder, "| `%s` | %s | %s | %s |\n", key, vi.Type, valueCell, description)
			} else {
				fmt.Fprintf(&builder, "| `%s` | %s | %s |\n", key, vi.Type, valueCell)
			}
		}

		builder.WriteString("\n")
	}

	if err := os.WriteFile(toPath, []byte(builder.String()), filePermission); err != nil {
		return coreerrs.Wrapf(err, "failed to write Markdown file %s", toPath)
	}

	return nil
}

// varInfo holds the inferred Go type and default value for a single
// environment variable, used when rendering Markdown tables.
type varInfo struct {
	Value string
	Type  string
}

// flattenConfigWithTypes recursively flattens config structure into env variables with type information.
func (conv *ConfigToMarkdownConverter) flattenConfigWithTypes(data map[string]any, prefix string) map[string]varInfo {
	result := make(map[string]varInfo)

	for key, value := range data {
		// Convert key to SCREAMING_SNAKE_CASE
		envKey := corestrings.ToScreamingSnakeCase(key)

		// Build full key with prefix
		fullKey := envKey
		if prefix != "" {
			fullKey = prefix + "__" + envKey
		}

		// Process value and detect type
		conv.processValueWithType(result, fullKey, value)
	}

	return result
}

// processValueWithType processes a single value, detects its type, and adds it to the result map.
func (conv *ConfigToMarkdownConverter) processValueWithType(result map[string]varInfo, key string, value any) {
	if value == nil {
		result[key] = varInfo{Value: "", Type: "string"}
		return
	}

	v := reflect.ValueOf(value)

	switch v.Kind() {
	case reflect.Map:
		// Nested map - recurse
		if mapValue, ok := value.(map[string]any); ok {
			nested := conv.flattenConfigWithTypes(mapValue, key)
			for k, vi := range nested {
				result[k] = vi
			}
		}

	case reflect.Slice, reflect.Array:
		// Array - format as [element1,element2,element3]
		arrayType := conv.detectArrayType(v)
		var arrayValue string
		if v.Len() == 0 {
			arrayValue = "[]"
		} else {
			var builder strings.Builder
			builder.WriteByte('[')
			for i := range v.Len() {
				if i > 0 {
					builder.WriteByte(',')
				}
				fmt.Fprintf(&builder, "%v", v.Index(i).Interface())
			}
			builder.WriteByte(']')
			arrayValue = builder.String()
		}
		result[key] = varInfo{Value: arrayValue, Type: arrayType}

	case reflect.String:
		result[key] = varInfo{Value: fmt.Sprintf("%v", value), Type: "string"}

	case reflect.Bool:
		result[key] = varInfo{Value: fmt.Sprintf("%v", value), Type: "bool"}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Check if it's a duration (ends with time unit)
		strValue := fmt.Sprintf("%v", value)
		if conv.isDuration(strValue) {
			result[key] = varInfo{Value: strValue, Type: "duration"}
		} else {
			result[key] = varInfo{Value: strValue, Type: "int"}
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		result[key] = varInfo{Value: fmt.Sprintf("%v", value), Type: "uint"}

	case reflect.Float32, reflect.Float64:
		result[key] = varInfo{Value: fmt.Sprintf("%v", value), Type: "float"}

	default:
		// For other types, use string representation
		result[key] = varInfo{Value: fmt.Sprintf("%v", value), Type: "string"}
	}
}

// detectArrayType detects the type of array elements.
func (conv *ConfigToMarkdownConverter) detectArrayType(v reflect.Value) string {
	if v.Len() == 0 {
		return "array"
	}

	// Check first element
	firstElem := v.Index(0)
	switch firstElem.Kind() {
	case reflect.String:
		return "[]string"
	case reflect.Bool:
		return "[]bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "[]int"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "[]uint"
	case reflect.Float32, reflect.Float64:
		return "[]float"
	case reflect.Map:
		return "[]map"
	default:
		return "array"
	}
}

// isDuration checks if a string value looks like a duration (e.g., "30s", "5m", "1h").
func (conv *ConfigToMarkdownConverter) isDuration(value string) bool {
	if value == "" {
		return false
	}
	// Check if string ends with time unit
	return strings.HasSuffix(value, "ns") ||
		strings.HasSuffix(value, "us") ||
		strings.HasSuffix(value, "ms") ||
		strings.HasSuffix(value, "s") ||
		strings.HasSuffix(value, "m") ||
		strings.HasSuffix(value, "h")
}

// groupBySection groups environment variable keys by their top-level section.
func (conv *ConfigToMarkdownConverter) groupBySection(keys []string) map[string][]string {
	sections := make(map[string][]string)

	for _, key := range keys {
		// Extract top-level section (everything before first __)
		parts := strings.Split(key, "__")
		sectionName := parts[0]

		sections[sectionName] = append(sections[sectionName], key)
	}

	return sections
}

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

const (
	sectionSeparatorLength    = 42
	minEntriesForMapOfStructs = 2 // Minimum entries to detect map[string]Struct pattern
)

var (
	sectionSeparator = "# " + strings.Repeat("=", sectionSeparatorLength) + "\n"
)

// ConfigToEnvConverter flattens YAML or TOML configuration trees into .env
// KEY=VALUE lines. Keys are built by joining SCREAMING_SNAKE_CASE field names
// with __ as the section delimiter. Output is grouped by top-level section
// with visual separators.
type ConfigToEnvConverter struct {
	parseComments bool
}

// NewConfigToEnvConverter creates a new config to .env converter.
func NewConfigToEnvConverter() *ConfigToEnvConverter {
	return &ConfigToEnvConverter{}
}

// SetParseComments enables or disables parsing of commented configuration values.
// When enabled, configuration values will be output as commented environment variables.
func (conv *ConfigToEnvConverter) SetParseComments(parse bool) {
	conv.parseComments = parse
}

// UncommentYAML strips leading # from lines that look like YAML key-value pairs
// or list items, leaving prose comments intact. Values starting with YAML-special
// characters (*, &, !, @, `) are auto-quoted to prevent parse errors.
func (conv *ConfigToEnvConverter) UncommentYAML(data []byte) []byte {
	return conv.uncommentYAML(data)
}

// Convert reads a YAML or TOML file at fromPath and writes the flattened
// .env output to toPath. When parseComments is enabled and format is "yaml",
// inline and block comments are tracked and emitted as commented env lines.
// #nosec G304 -- CLI tool, path from command-line arguments
func (conv *ConfigToEnvConverter) Convert(fromPath, toPath, format string) error {
	// Read file
	data, err := os.ReadFile(fromPath)
	if err != nil {
		return coreerrs.Wrapf(err, "failed to read config file %s", fromPath)
	}

	// Parse based on format and extract comments if needed
	var configData map[string]any
	var commentedKeys map[string]bool

	if conv.parseComments && format == formatYAML {
		configData, commentedKeys, err = conv.parseYAMLWithComments(data)
		if err != nil {
			return err
		}
	} else {
		switch format {
		case formatYAML:
			if err := yaml.Unmarshal(data, &configData); err != nil {
				return coreerrs.WrapOperation(err, "parse YAML")
			}
		case formatTOML:
			if err := toml.Unmarshal(data, &configData); err != nil {
				return coreerrs.WrapOperation(err, "parse TOML")
			}
		default:
			return fmt.Errorf("unsupported format: %s", format)
		}
	}

	return conv.ConvertDataWithComments(configData, commentedKeys, toPath)
}

// ConvertData flattens pre-loaded configData and writes it to toPath in .env format.
// No comment tracking is performed; all lines are emitted as active variables.
func (conv *ConfigToEnvConverter) ConvertData(configData map[string]any, toPath string) error {
	return conv.ConvertDataWithComments(configData, nil, toPath)
}

// ConvertDataWithComments flattens configData into .env format and writes it
// to toPath. Keys present in commentedKeys are prefixed with # in the output.
// Pass a nil commentedKeys map to emit all variables as active.
func (conv *ConfigToEnvConverter) ConvertDataWithComments(configData map[string]any, commentedKeys map[string]bool, toPath string) error {
	// Flatten to env vars
	envVars := conv.FlattenConfig(configData, "")

	// Sort keys for consistent output
	keys := make([]string, 0, len(envVars))
	for key := range envVars {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	// Group keys by top-level section
	sections := conv.groupBySection(keys)

	// Write .env file with sections
	var builder strings.Builder
	isFirst := true
	for _, sectionName := range getSortedSectionNames(sections) {
		sectionKeys := sections[sectionName]

		// Add section separator (except for first section)
		if !isFirst {
			builder.WriteString("\n")
		}
		isFirst = false

		// Add section header
		builder.WriteString(sectionSeparator)
		fmt.Fprintf(&builder, "# %s Configuration\n", formatSectionName(sectionName))
		builder.WriteString(sectionSeparator)

		// Add variables in this section
		for _, key := range sectionKeys {
			value := envVars[key]
			// If parseComments is enabled and key is marked as commented, add # prefix
			if conv.parseComments && commentedKeys != nil && commentedKeys[key] {
				fmt.Fprintf(&builder, "# %s=%s\n", key, value)
			} else {
				fmt.Fprintf(&builder, "%s=%s\n", key, value)
			}
		}
	}

	if err := os.WriteFile(toPath, []byte(builder.String()), filePermission); err != nil {
		return coreerrs.Wrapf(err, "failed to write .env file %s", toPath)
	}

	return nil
}

// groupBySection groups environment variable keys by their top-level section.
func (conv *ConfigToEnvConverter) groupBySection(keys []string) map[string][]string {
	sections := make(map[string][]string)

	for _, key := range keys {
		// Extract top-level section (everything before first __)
		parts := strings.Split(key, "__")
		sectionName := parts[0]

		sections[sectionName] = append(sections[sectionName], key)
	}

	return sections
}

// getSortedSectionNames returns section names sorted alphabetically.
func getSortedSectionNames(sections map[string][]string) []string {
	names := make([]string, 0, len(sections))
	for name := range sections {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// formatSectionName converts SCREAMING_SNAKE_CASE to Title Case for display.
func formatSectionName(s string) string {
	// Convert SCREAMING_SNAKE_CASE to Title Case
	// E.g., "GRPC_SERVER" -> "Grpc Server"
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if part != "" {
			// Capitalize first letter, lowercase the rest
			parts[i] = strings.ToUpper(string(part[0])) + strings.ToLower(part[1:])
		}
	}
	return strings.Join(parts, " ")
}

// parseYAMLWithComments parses YAML and tracks which keys have line comments.
// It handles both inline comments and fully commented-out YAML blocks.
func (conv *ConfigToEnvConverter) parseYAMLWithComments(data []byte) (configData map[string]any, commentedKeys map[string]bool, err error) {
	// First, try to uncomment the YAML to parse it
	uncommentedData := conv.uncommentYAML(data)

	var node yaml.Node
	if err = yaml.Unmarshal(uncommentedData, &node); err != nil {
		return nil, nil, coreerrs.WrapOperation(err, "parse YAML")
	}

	configData = make(map[string]any)
	commentedKeys = make(map[string]bool)

	// Unmarshal into regular map
	if err = yaml.Unmarshal(uncommentedData, &configData); err != nil {
		return nil, nil, coreerrs.WrapOperation(err, "parse YAML data")
	}

	// Check if this was a fully commented YAML file
	// by comparing original data with uncommented data
	if conv.isFullyCommented(data) {
		// Mark all keys as commented
		conv.markAllKeysAsCommented(configData, "", commentedKeys)
	} else {
		// Traverse node to find keys with inline comments
		conv.findCommentedKeys(&node, "", commentedKeys)
	}

	return configData, commentedKeys, nil
}

// uncommentYAML removes leading # from YAML lines to parse commented configurations.
func (conv *ConfigToEnvConverter) uncommentYAML(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	uncommented := make([]string, 0, len(lines))

	for _, line := range lines {
		uncommented = append(uncommented, conv.processLine(line))
	}

	return []byte(strings.Join(uncommented, "\n"))
}

// processLine processes a single line, uncommenting if it's YAML syntax.
func (conv *ConfigToEnvConverter) processLine(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	leadingSpace := line[:len(line)-len(trimmed)]

	afterHash, ok := strings.CutPrefix(trimmed, "#")
	if !ok {
		return line
	}
	afterHashTrimmed := strings.TrimLeft(afterHash, " \t")

	if conv.isYAMLSyntax(afterHashTrimmed) || afterHash == "" {
		uncommented := leadingSpace + afterHash
		return conv.quoteYAMLSpecialValues(uncommented)
	}

	return line
}

// yamlSpecialValuePrefixes are characters that have special meaning at the start
// of a YAML scalar value and cause parse errors when left unquoted.
const yamlSpecialValuePrefixes = "*&!@`"

// quoteYAMLSpecialValues wraps values starting with YAML-special characters
// in double quotes to prevent parse errors after uncommenting.
func (conv *ConfigToEnvConverter) quoteYAMLSpecialValues(line string) string {
	trimmed := strings.TrimLeft(line, " \t")

	colonIdx := strings.Index(trimmed, ":")
	if colonIdx <= 0 {
		return line
	}

	afterColon := trimmed[colonIdx+1:]
	value := strings.TrimLeft(afterColon, " \t")
	if len(value) == 0 {
		return line
	}

	// Already quoted
	if value[0] == '"' || value[0] == '\'' {
		return line
	}

	// Check if value starts with a YAML-special character
	if !strings.ContainsRune(yamlSpecialValuePrefixes, rune(value[0])) {
		return line
	}

	leadingSpace := line[:len(line)-len(trimmed)]
	key := trimmed[:colonIdx]
	quotedValue := `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`

	return leadingSpace + key + ": " + quotedValue
}

// isYAMLSyntax checks if the content matches YAML syntax patterns.
func (conv *ConfigToEnvConverter) isYAMLSyntax(content string) bool {
	if content == "" {
		return false
	}
	return conv.isYAMLListItem(content) || conv.isYAMLKeyValue(content)
}

// isYAMLListItem checks if content is a YAML list item (starts with -).
func (conv *ConfigToEnvConverter) isYAMLListItem(content string) bool {
	return strings.HasPrefix(content, "-")
}

// isYAMLKeyValue checks if content matches key:value pattern with valid YAML key.
func (conv *ConfigToEnvConverter) isYAMLKeyValue(content string) bool {
	colonIdx := strings.Index(content, ":")
	if colonIdx <= 0 {
		return false
	}
	return conv.isValidYAMLKey(content[:colonIdx])
}

// isValidYAMLKey checks if a string is a valid YAML key (alphanumeric, _, -).
func (conv *ConfigToEnvConverter) isValidYAMLKey(key string) bool {
	for _, r := range key {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') &&
			(r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

// isFullyCommented checks if the YAML file is fully commented out.
func (conv *ConfigToEnvConverter) isFullyCommented(data []byte) bool {
	lines := strings.Split(string(data), "\n")
	hasContent := false
	hasCommentedYAML := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "# ") {
			continue // Empty line or regular comment
		}
		if strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, ":") {
			hasCommentedYAML = true
		} else if strings.Contains(trimmed, ":") {
			hasContent = true
		}
	}

	return hasCommentedYAML && !hasContent
}

// markAllKeysAsCommented recursively marks all keys in config as commented.
func (conv *ConfigToEnvConverter) markAllKeysAsCommented(data map[string]any, prefix string, commentedKeys map[string]bool) {
	for key, value := range data {
		envKey := corestrings.ToScreamingSnakeCase(key)
		fullKey := envKey
		if prefix != "" {
			fullKey = prefix + "__" + envKey
		}

		commentedKeys[fullKey] = true

		// Recurse into nested maps
		if mapValue, ok := value.(map[string]any); ok {
			conv.markAllKeysAsCommented(mapValue, fullKey, commentedKeys)
		}
	}
}

// findCommentedKeys recursively finds keys that have line comments or head comments.
func (conv *ConfigToEnvConverter) findCommentedKeys(node *yaml.Node, prefix string, commentedKeys map[string]bool) {
	if node == nil {
		return
	}

	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		conv.findCommentedKeys(node.Content[0], prefix, commentedKeys)
		return
	}

	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content)-1; i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			key := keyNode.Value
			fullKey := corestrings.ToScreamingSnakeCase(key)
			if prefix != "" {
				fullKey = prefix + "__" + fullKey
			}

			// Check if the value node has a line comment or head comment
			if valueNode.LineComment != "" || valueNode.HeadComment != "" {
				commentedKeys[fullKey] = true
			}

			// Recurse into nested maps
			if valueNode.Kind == yaml.MappingNode {
				conv.findCommentedKeys(valueNode, fullKey, commentedKeys)
			}
		}
	}
}

// estimateSize provides a rough estimate of the final map size for nested structures.
// This helps reduce map reallocations during recursive flattening.
func estimateSize(data map[string]any) int {
	// Rough estimate: assume 2x the top-level keys for nested structures
	return len(data) * 2 //nolint:mnd
}

// FlattenConfig recursively flattens a nested config map into a flat
// map[envKey]value suitable for .env output. prefix is prepended (with __)
// to all generated keys; pass "" for the top level.
//
// Map-of-structs patterns (e.g., consumers with hyphenated instance names)
// are detected automatically and their keys are preserved verbatim rather
// than converted to SCREAMING_SNAKE_CASE.
func (conv *ConfigToEnvConverter) FlattenConfig(data map[string]any, prefix string) map[string]string {
	// Estimate size and create map with capacity
	result := make(map[string]string, estimateSize(data))
	conv.flattenConfigInto(result, data, prefix)
	return result
}

// flattenConfigInto recursively flattens config structure into an existing result map.
// This avoids allocating new maps for each recursive call, improving performance.
func (conv *ConfigToEnvConverter) flattenConfigInto(result map[string]string, data map[string]any, prefix string) {
	// Check if this is a map of structs (e.g., map[string]*NatsConsumer)
	isMapOfStructs := conv.isMapOfStructs(data)

	for key, value := range data {
		var envKey string
		if isMapOfStructs {
			// For map keys (like "keycloak-sync"), preserve original format
			// but replace hyphens with underscores and uppercase for env var compatibility
			envKey = strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
		} else {
			// For struct fields, convert to SCREAMING_SNAKE_CASE
			envKey = corestrings.ToScreamingSnakeCase(key)
		}

		// Build full key with prefix
		fullKey := envKey
		if prefix != "" {
			fullKey = prefix + "__" + envKey
		}

		// Handle different value types
		conv.processValueInto(result, fullKey, value)
	}
}

// isMapOfStructs checks if the map represents a map[string]Struct pattern.
// This is true when all values are map[string]any AND at least 2 entries exist,
// suggesting this is a collection of similar objects (e.g., consumers, users).
func (conv *ConfigToEnvConverter) isMapOfStructs(data map[string]any) bool {
	if len(data) == 0 {
		return false
	}

	// Check if all values are map[string]any
	var firstKeys map[string]bool
	structCount := 0
	hasMultipleEntries := len(data) >= minEntriesForMapOfStructs

	// Also check if any key contains characters typical for instance names
	// (hyphens, dots, numbers) rather than field names (camelCase)
	hasInstanceNamePattern := false

	for key, value := range data {
		mapValue, ok := value.(map[string]any)
		if !ok {
			// Not a map, so this is not a map of structs
			return false
		}

		structCount++

		// Check if key looks like an instance name (contains hyphen, dot, or starts with number)
		if strings.ContainsAny(key, "-_.") || (key != "" && key[0] >= '0' && key[0] <= '9') {
			hasInstanceNamePattern = true
		}

		// Collect keys from first struct
		if firstKeys == nil {
			firstKeys = make(map[string]bool)
			for k := range mapValue {
				firstKeys[k] = true
			}
		}
	}

	// This is a map of structs if:
	// 1. All values are struct-like (map[string]any)
	// 2. Either has multiple entries OR keys look like instance names
	// Examples:
	//   - consumers: {"keycloak-sync": {...}, "user-events": {...}} -> true
	//   - nats: {"hosts": [...], "consumers": {...}} -> false (single config struct)
	return structCount > 0 && (hasMultipleEntries || hasInstanceNamePattern)
}

// processValueInto processes a single value and adds it to the result map.
// This version reuses the result map instead of allocating new ones.
func (conv *ConfigToEnvConverter) processValueInto(result map[string]string, key string, value any) {
	if value == nil {
		return
	}

	v := reflect.ValueOf(value)

	switch v.Kind() {
	case reflect.Map:
		if mapValue, ok := value.(map[string]any); ok {
			// Recursively flatten into the existing result map
			conv.flattenConfigInto(result, mapValue, key)
		}
	case reflect.Slice, reflect.Array:
		result[key] = conv.formatArrayValue(v)
	default:
		result[key] = fmt.Sprintf("%v", value)
	}
}

// formatArrayValue formats a slice or array as [element1,element2,element3].
func (conv *ConfigToEnvConverter) formatArrayValue(v reflect.Value) string {
	if v.Len() == 0 {
		return "[]"
	}

	var builder strings.Builder
	builder.WriteByte('[')

	for i := range v.Len() {
		if i > 0 {
			builder.WriteByte(',')
		}
		fmt.Fprintf(&builder, "%v", v.Index(i).Interface())
	}

	builder.WriteByte(']')
	return builder.String()
}

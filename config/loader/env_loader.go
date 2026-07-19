// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import (
	"reflect"
	"slices"
	"strconv"
	"strings"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// loadEnvs loads configuration values from environment variables.
// It maps struct field names to environment variable names using the configured prefix and delimiter.
// Supports array element access using the configured delimiter.
// If a secrets manager is configured, it also expands $__secret{namespace:key} placeholders.
func (cf *Config) loadEnvs() error {
	if cf.options.skipEnv {
		return nil
	}

	var err error

	// Use envsWithSecrets to support secret expansion in environment variables
	ctx, cancel := cf.options.getSecretsContext()
	envs, err := envsWithSecrets(ctx, cf.options.secretsManager, cf.options.strict)
	cancel()
	if err != nil {
		return coreerrs.WrapOperation(err, "expand secrets in environment variables")
	}

	// First, handle regular field mappings
	cf.fields.each(func(fld *field) bool {
		envName := cf.envFieldName(fld)
		if envName == "" {
			return true
		}

		if val, ok := envs[envName]; ok {
			if err = fld.setValue(val, envTagName, cf.options.strict); err != nil {
				return false
			}
		}

		return true
	})

	if err != nil {
		return err
	}

	// Then, handle array element mappings using configured delimiter
	if err = cf.loadArrayEnvs(envs); err != nil {
		return err
	}

	// Finally, handle nested structs using configured delimiter
	if err = cf.loadNestedEnvs(envs); err != nil {
		return err
	}

	return nil
}

// loadArrayEnvs handles array element environment variables using configured delimiter.
func (cf *Config) loadArrayEnvs(envs map[string]string) error {
	// Group environment variables by array pattern
	arrayEnvs := cf.parseArrayEnvs(envs)

	// Sort array paths by depth (number of delimiters) to process parent arrays first
	arrayPaths := make([]string, 0, len(arrayEnvs))
	for fieldPath := range arrayEnvs {
		arrayPaths = append(arrayPaths, fieldPath)
	}

	// Sort by depth (fewer delimiters = shallower = process first)
	delimiter := cf.options.envSectionDelimiter
	slices.SortStableFunc(arrayPaths, func(a, b string) int {
		return strings.Count(a, delimiter) - strings.Count(b, delimiter)
	})

	// Process each array pattern in order of depth
	for _, fieldPath := range arrayPaths {
		elements := arrayEnvs[fieldPath]
		if err := cf.setArrayElements(fieldPath, elements); err != nil {
			return err
		}
	}

	return nil
}

// parseArrayEnvs parses environment variables to find array patterns.
// Returns a map where key is the field path and value is a map
// of element paths to their values using configured section delimiter.
func (cf *Config) parseArrayEnvs(envs map[string]string) map[string]map[string]string {
	arrayEnvs := make(map[string]map[string]string)

	for envKey, envValue := range envs {
		cf.processArrayEnvKey(envKey, envValue, arrayEnvs)
	}

	return arrayEnvs
}

// processArrayEnvKey processes a single environment key for array patterns.
func (cf *Config) processArrayEnvKey(envKey, envValue string, result map[string]map[string]string) {
	parts := cf.parseArrayEnvKey(envKey)
	if len(parts) < minEnvKeyParts {
		return
	}

	arrayPositions := cf.findArrayIndices(parts)
	if len(arrayPositions) == 0 {
		return
	}

	cf.extractArrayFields(parts, arrayPositions, envValue, result)
}

// findArrayIndices locates all array index positions in parts.
func (cf *Config) findArrayIndices(parts []string) []int {
	positions := make([]int, 0)

	for i := 1; i < len(parts); i++ {
		positions = coreslices.AppendIf(positions, cf.isArrayIndex(parts[i]), i)
	}

	return positions
}

// isArrayIndex checks if a part represents an array index.
// Returns true if the part is either a pure number (e.g., "0", "1")
// or has an index suffix (e.g., "BACK_OFF_0").
func (cf *Config) isArrayIndex(part string) bool {
	// Check if this is a pure number
	if _, err := strconv.Atoi(part); err == nil {
		return true
	}

	// Check if this has an index suffix
	_, _, hasIndex := splitFieldNameAndIndex(part)
	return hasIndex
}

// extractArrayFields extracts field paths from array patterns.
func (cf *Config) extractArrayFields(parts []string, positions []int, envValue string, result map[string]map[string]string) {
	delimiter := cf.options.envSectionDelimiter

	// Pre-compute joined paths to avoid repeated string allocations
	pathPrefixCache := make(map[int]string, len(parts))
	pathSuffixCache := make(map[int]string, len(parts))

	for _, position := range positions {
		if position == 0 {
			continue
		}

		arrayField, elementPath := cf.buildArrayPaths(parts, position, delimiter, pathPrefixCache, pathSuffixCache)

		if result[arrayField] == nil {
			result[arrayField] = make(map[string]string)
		}
		result[arrayField][elementPath] = envValue
	}
}

// buildArrayPaths builds the array field path and element path for a given position.
func (cf *Config) buildArrayPaths(
	parts []string,
	position int,
	delimiter string,
	prefixCache, suffixCache map[int]string,
) (arrayField, elementPath string) {
	partWithIndex := parts[position]

	// Check if this part has an index suffix (e.g., "BACK_OFF_0")
	if baseName, _, hasIndex := splitFieldNameAndIndex(partWithIndex); hasIndex {
		return cf.buildIndexedFieldPaths(parts, position, baseName, partWithIndex, delimiter, prefixCache)
	}

	// This is a pure numeric index
	return cf.buildNumericIndexPaths(parts, position, delimiter, prefixCache, suffixCache)
}

// buildIndexedFieldPaths builds paths for parts with index suffixes (e.g., "BACK_OFF_0").
func (cf *Config) buildIndexedFieldPaths(
	parts []string,
	position int,
	baseName, partWithIndex, delimiter string,
	prefixCache map[int]string,
) (arrayField, elementPath string) {
	prefix := cf.getCachedPrefix(parts, position, delimiter, prefixCache)

	arrayField = prefix
	if arrayField != "" {
		arrayField += delimiter
	}
	arrayField += baseName

	elementPath = partWithIndex // Keep the full part with index for element path

	return arrayField, elementPath
}

// buildNumericIndexPaths builds paths for pure numeric indices.
func (cf *Config) buildNumericIndexPaths(
	parts []string,
	position int,
	delimiter string,
	prefixCache, suffixCache map[int]string,
) (arrayField, elementPath string) {
	prefix := cf.getCachedPrefix(parts, position, delimiter, prefixCache)
	suffix := cf.getCachedSuffix(parts, position, delimiter, suffixCache)

	return prefix, suffix
}

// getCachedPrefix returns the cached prefix or computes and caches it.
func (cf *Config) getCachedPrefix(parts []string, position int, delimiter string, cache map[int]string) string {
	if prefix, ok := cache[position]; ok {
		return prefix
	}

	prefix := strings.Join(parts[0:position], delimiter)
	cache[position] = prefix
	return prefix
}

// getCachedSuffix returns the cached suffix or computes and caches it.
func (cf *Config) getCachedSuffix(parts []string, position int, delimiter string, cache map[int]string) string {
	if suffix, ok := cache[position]; ok {
		return suffix
	}

	suffix := strings.Join(parts[position:], delimiter)
	cache[position] = suffix
	return suffix
}

// parseArrayEnvKey parses environment variable key to check for array pattern.
// Returns parts if it matches array pattern using configured section delimiter.
func (cf *Config) parseArrayEnvKey(envKey string) []string {
	parts := strings.Split(envKey, cf.options.envSectionDelimiter)
	if len(parts) < minEnvKeyParts {
		return nil
	}

	// Check if any part is a number (array index) or ends with _<number>
	for _, part := range parts[1:] {
		// Check if this part is a pure number
		if _, err := strconv.Atoi(part); err == nil {
			// Found array index, return from this position
			return parts
		}

		// Check if this part ends with _<number> (e.g., "BACK_OFF_0")
		if _, _, hasIndex := splitFieldNameAndIndex(part); hasIndex {
			// Found array index in field name, return from this position
			return parts
		}
	}

	return nil
}

// setArrayElements sets values for array elements based on parsed environment variables.
func (cf *Config) setArrayElements(fieldPath string, elements map[string]string) error {
	// Try to find it dynamically in the structure (this will ensure parent structs are initialized)
	arrayField := cf.findFieldByPath(fieldPath)

	// If not found dynamically, try to find in existing static fields
	if arrayField == nil {
		cf.fields.each(func(fld *field) bool {
			if cf.matchesArrayField(fld, fieldPath) {
				arrayField = fld
				return false // Stop searching
			}
			return true
		})
	}

	if arrayField == nil {
		// Do not error here even in strict mode: parseArrayEnvs processes ALL env vars
		// that match array patterns, including system env vars that are not targeting
		// the config struct.
		return nil
	}

	// Check if the field value is valid
	if !arrayField.value.IsValid() {
		return nil
	}

	// Check if this is actually a slice/array field
	fieldType := arrayField.value.Type()
	if fieldType.Kind() != reflect.Slice {
		return nil
	}

	return cf.setSliceElements(arrayField, elements)
}

// matchesArrayField checks if a field matches the array field path from environment variable.
func (cf *Config) matchesArrayField(fld *field, fieldPath string) bool {
	// Use the same logic as envFieldName to get the expected environment name
	expectedEnvName := cf.envFieldName(fld)
	return expectedEnvName == fieldPath
}

// setSliceElements sets individual elements in a slice based on environment variables.
func (cf *Config) setSliceElements(arrayField *field, elements map[string]string) error {
	sliceType := arrayField.value.Type()
	elementType := sliceType.Elem()

	// Check if this is a slice of primitives or a slice of structs
	isPrimitiveSlice := elementType.Kind() != reflect.Struct &&
		(elementType.Kind() != reflect.Pointer || elementType.Elem().Kind() != reflect.Struct)

	if isPrimitiveSlice {
		// Handle slice of primitives (e.g., []string, []int)
		return cf.setPrimitiveSliceElements(arrayField, elements)
	}

	// Handle slice of structs
	return cf.setStructSliceElements(arrayField, elements)
}

// setPrimitiveSliceElements sets elements in a slice of primitive types.
func (cf *Config) setPrimitiveSliceElements(arrayField *field, elements map[string]string) error {
	// Group elements by index (should be just index -> value for primitives)
	indexedValues := make(map[int]string)
	maxIndex := -1

	for elementPath, value := range elements {
		var index int
		var err error

		// Try to parse as pure number first
		index, err = strconv.Atoi(elementPath)
		if err != nil {
			// Not a pure number, check if it's a field name with index (e.g., "BACK_OFF_0" or "BACK_OFF[0]")
			if _, indexOrKey, hasIndex := splitFieldNameAndIndex(elementPath); hasIndex {
				var isNumeric bool
				index, isNumeric = parseIndexOrKey(indexOrKey)
				if !isNumeric {
					continue
				}
			} else {
				continue
			}
		}

		if index > maxIndex {
			maxIndex = index
		}

		indexedValues[index] = value
	}

	if maxIndex < 0 {
		return nil
	}

	// Ensure slice is large enough
	if err := cf.ensureSliceSize(arrayField, maxIndex+1); err != nil {
		return err
	}

	// Set values for each element
	for index, value := range indexedValues {
		elementValue := arrayField.value.Index(index)
		if err := set(elementValue, value, false, true, cf.options.strict); err != nil {
			return err
		}
	}

	return nil
}

// setStructSliceElements sets elements in a slice of struct types.
func (cf *Config) setStructSliceElements(arrayField *field, elements map[string]string) error {
	// Group elements by index
	indexedElements := make(map[int]map[string]string)
	maxIndex := -1
	delimiter := cf.options.envSectionDelimiter

	for elementPath, value := range elements {
		parts := strings.Split(elementPath, delimiter)
		if len(parts) < minEnvKeyParts {
			continue
		}

		index, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		if index > maxIndex {
			maxIndex = index
		}

		fieldPath := strings.Join(parts[1:], delimiter)

		if indexedElements[index] == nil {
			indexedElements[index] = make(map[string]string)
		}
		indexedElements[index][fieldPath] = value
	}

	if maxIndex < 0 {
		return nil
	}

	// Ensure slice is large enough
	if err := cf.ensureSliceSize(arrayField, maxIndex+1); err != nil {
		return err
	}

	// Set values for each element
	for index, fieldValues := range indexedElements {
		elementValue := arrayField.value.Index(index)
		if err := cf.setStructFields(elementValue, fieldValues); err != nil {
			return err
		}
	}

	return nil
}

// ensureSliceSize ensures that a field's slice has at least the specified size.
// It delegates to ensureSliceSizeForValue and handles map updates if needed.
func (cf *Config) ensureSliceSize(arrayField *field, minSize int) error {
	// Use the base function to resize the slice
	if err := cf.ensureSliceSizeForValue(arrayField.value, minSize); err != nil {
		return err
	}

	// If this field is from a map, we need to update the map value
	if arrayField.parentMap != nil && arrayField.mapKey != nil && arrayField.parentValue != nil {
		// The parent struct containing the modified slice needs to be saved back to the map
		// parentValue is either a pointer to the struct or the struct itself
		valueToSet := *arrayField.parentValue

		// Update the map with the modified value
		// Since we modified arrayField.value (which is a field inside the struct),
		// and the struct is either pointed to by parentValue (if pointer) or is parentValue (if not pointer),
		// the changes are already reflected in parentValue
		arrayField.parentMap.SetMapIndex(*arrayField.mapKey, valueToSet)
	}

	return nil
}

// setStructFields sets fields of a struct based on a map of field names to values.
// Supports nested field paths using configured delimiter.
func (cf *Config) setStructFields(structValue reflect.Value, fieldValues map[string]string) error {
	if structValue.Kind() == reflect.Pointer {
		if structValue.IsNil() {
			structValue.Set(reflect.New(structValue.Type().Elem()))
		}
		structValue = structValue.Elem()
	}

	if structValue.Kind() != reflect.Struct {
		return nil
	}

	delimiter := cf.options.envSectionDelimiter
	for fieldPath, value := range fieldValues {
		parts := strings.Split(fieldPath, delimiter)
		if err := cf.setNestedStructField(structValue, parts, value); err != nil {
			return err
		}
	}

	return nil
}

// ensureSliceSizeForValue ensures a slice has at least the specified size.
func (cf *Config) ensureSliceSizeForValue(sliceValue reflect.Value, minSize int) error {
	currentLen := sliceValue.Len()
	if currentLen >= minSize {
		return nil
	}

	// Create a new slice with the required size
	sliceType := sliceValue.Type()
	elementType := sliceType.Elem()

	newSlice := reflect.MakeSlice(sliceType, minSize, minSize)

	// Copy existing elements
	for i := range currentLen {
		newSlice.Index(i).Set(sliceValue.Index(i))
	}

	// Initialize new elements
	for i := currentLen; i < minSize; i++ {
		newElement := reflect.New(elementType).Elem()
		if elementType.Kind() == reflect.Struct {
			newSlice.Index(i).Set(newElement)
		} else {
			newSlice.Index(i).Set(reflect.Zero(elementType))
		}
	}

	sliceValue.Set(newSlice)
	return nil
}

// setFieldValue sets a reflect.Value from a string value.
func (cf *Config) setFieldValue(fieldValue reflect.Value, value string) error {
	return set(fieldValue, value, false, true, cf.options.strict)
}

// loadNestedEnvs handles nested structures using the section delimiter.
func (cf *Config) loadNestedEnvs(envs map[string]string) error {
	for envKey, envValue := range envs {
		// Only process environment variables that contain the section delimiter
		// which indicates nested struct levels
		if strings.Contains(envKey, cf.options.envSectionDelimiter) {
			if err := cf.setNestedFieldFromEnv(envKey, envValue); err != nil {
				return err
			}
		}
	}
	return nil
}

// setNestedFieldFromEnv sets a nested field value using the section delimiter.
func (cf *Config) setNestedFieldFromEnv(envKey, envValue string) error {
	// Use the section delimiter to split nested struct levels
	delimiter := cf.options.envSectionDelimiter
	parts := strings.Split(envKey, delimiter)

	if len(parts) < minEnvKeyParts {
		return nil
	}

	// Remove prefix if present
	if cf.options.envPrefix != "" {
		if rest, ok := strings.CutPrefix(envKey, cf.options.envPrefix); ok {
			envKey = rest
			parts = strings.Split(envKey, delimiter)
		}
	}

	// Find the root field
	rootFieldName := parts[0]
	var rootField *field

	cf.fields.each(func(fld *field) bool {
		// Check if this field matches the root field name
		// Only match root-level fields (no parent)
		if fld.parent == nil && cf.matchesNestedField(fld, rootFieldName) {
			rootField = fld
			return false // Stop searching
		}
		return true
	})

	// If not found in fields list, try to find it directly in the config struct
	// This handles fields with default:"-" tag that haven't been initialized yet
	if rootField == nil {
		rootField = cf.findRootFieldByName(rootFieldName)
	}

	if rootField == nil {
		// Do not error here even in strict mode: loadNestedEnvs processes ALL env vars
		// containing the section delimiter (e.g. "__"), including system env vars
		// (like __CFBundleIdentifier on macOS) that are not targeting the config struct.
		return nil
	}

	// Navigate to the nested field and set its value
	return cf.setNestedFieldValue(rootField, parts[1:], envValue)
}

// findRootFieldByName finds a root-level field in the config struct by name.
// This is used to find fields with default:"-" that haven't been initialized yet.
func (cf *Config) findRootFieldByName(fieldName string) *field {
	if cf.conf == nil {
		return nil
	}

	configValue := reflect.ValueOf(cf.conf)
	if configValue.Kind() == reflect.Pointer {
		configValue = configValue.Elem()
	}

	if configValue.Kind() != reflect.Struct {
		return nil
	}

	configType := configValue.Type()

	// Search for the field in the root struct
	for i := range configType.NumField() {
		structField := configType.Field(i)

		// Check if field matches using the same logic as matchesNestedField
		if cf.fieldMatchesByName(structField, fieldName) {
			fieldValue := configValue.Field(i)

			// If it's a pointer and nil, initialize it
			if fieldValue.Kind() == reflect.Pointer && fieldValue.IsNil() {
				fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
			}

			// Create a field descriptor
			tags := make(map[string]string)
			tags[cf.options.structTag] = structField.Tag.Get(cf.options.structTag)
			tags[envTagName] = structField.Tag.Get(envTagName)
			tags[defaultValueTagName] = structField.Tag.Get(defaultValueTagName)

			return &field{
				name:     structField.Name,
				fullName: structField.Name,
				value:    fieldValue,
				field:    structField,
				tags:     tags,
			}
		}
	}

	return nil
}

// fieldNameMatches checks if a field matches the given name based on tags or field name.
// This is a helper function used by other field matching functions to avoid code duplication.
//
// Previously the comparisons went through corestrings.InternLowerString
// — a GLOBAL LRU cache. A process that ingests untrusted env keys
// (containers, sidecars, plugin hosts) churned the global interner
// and evicted legitimately-hot strings used elsewhere in the binary.
// Plain strings.ToLower allocates one short string per comparison
// (config-load-time, not a hot path) but keeps the global interner
// reserved for callers that actually benefit from pointer-identity.
func (cf *Config) fieldNameMatches(structTag, envTag, fieldName, targetName string) bool {
	targetLower := strings.ToLower(targetName)

	// Check env tag first (highest priority)
	if envTag != "" && strings.ToLower(envTag) == targetLower {
		return true
	}

	// Check struct tag (yaml, json, etc.)
	if structTag != "" {
		convertedTag := corestrings.ToScreamingSnakeCase(structTag)
		if strings.ToLower(convertedTag) == targetLower {
			return true
		}
	}

	// Convert field name to SCREAMING_SNAKE_CASE for comparison
	convertedFieldName := corestrings.ToScreamingSnakeCase(fieldName)
	return strings.ToLower(convertedFieldName) == targetLower
}

// fieldMatchesByName checks if a struct field matches the given name.
func (cf *Config) fieldMatchesByName(field reflect.StructField, name string) bool {
	return cf.fieldNameMatches(
		field.Tag.Get(cf.options.structTag),
		field.Tag.Get(envTagName),
		field.Name,
		name,
	)
}

// matchesNestedField checks if a field matches the nested field name from environment variable.
func (cf *Config) matchesNestedField(fld *field, fieldName string) bool {
	return cf.fieldNameMatches(
		fld.tags[cf.options.structTag],
		fld.tags[envTagName],
		fld.name,
		fieldName,
	)
}

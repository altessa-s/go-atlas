// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import (
	"cmp"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// loadEnvs loads configuration values from environment variables.
// With a prefix configured, only the variables carrying it are read, and the
// prefix is stripped before their names are matched against the fields.
// It maps struct field names to environment variable names using the configured delimiter.
// Supports array element access using the configured delimiter.
// If a secrets manager is configured, it also expands $__secret{namespace:key} placeholders.
func (cf *Config) loadEnvs() error {
	if cf.options.skipEnv {
		return nil
	}

	var err error

	// Use envsWithSecrets to support secret expansion in environment variables.
	ctx, cancel := cf.options.getSecretsContext()
	envs, err := envsWithSecrets(ctx, cf.options.secretsManager, cf.options.envPrefix, cf.options.envSectionDelimiter, cf.options.strict)
	cancel()
	if err != nil {
		return coreerrs.WrapOperation(err, "expand secrets in environment variables")
	}

	// The fields an env tag names, scanned for every variable name below.
	cf.envTagged = cf.collectEnvTagged()
	defer func() { cf.envTagged = nil }()

	// First, handle regular field mappings. A field under a nil pointer
	// struct is reached by allocating the structs along its path, only when
	// the environment sets it. When several names of a field are set, the
	// struct tag spelling wins (see preferredEnvName); every name of the field
	// is consumed, so the other passes do not set it again.
	root := reflect.ValueOf(cf.conf).Elem()
	var consumed map[string]struct{}
	cf.fields.each(func(fld *field) bool {
		names := cf.envFieldNames(fld)
		envName, ok := cf.preferredEnvName(names, envs)
		if !ok {
			return true
		}
		value := fld.value
		if !value.IsValid() {
			value = fieldByIndexAlloc(root, fld.index)
		}
		err = cf.envVarError(envName, cf.setFieldValue(value, envs[envName], fld.pathSteps()))
		if consumed == nil {
			consumed = make(map[string]struct{})
		}
		for _, name := range names {
			consumed[name] = struct{}{}
		}

		return err == nil
	})

	if err != nil {
		return err
	}

	// The other passes read only the variables left whose names contain the
	// section delimiter, in name order. Of those that set the same value in
	// different spellings, only the winner is applied.
	keys := cf.dropShadowedEnvs(cf.nestedEnvKeys(envs, consumed))

	// Then, handle array element mappings using configured delimiter
	if err = cf.loadArrayEnvs(envs, keys); err != nil {
		return err
	}

	// Finally, handle nested structs using configured delimiter
	if err = cf.loadNestedEnvs(envs, keys); err != nil {
		return err
	}

	return nil
}

// nestedEnvKeys returns, sorted, the names in envs that contain the section
// delimiter, except the ones in consumed.
func (cf *Config) nestedEnvKeys(envs map[string]string, consumed map[string]struct{}) []string {
	var keys []string
	for envKey := range envs {
		if _, done := consumed[envKey]; !done && strings.Contains(envKey, cf.options.envSectionDelimiter) {
			keys = append(keys, envKey)
		}
	}
	slices.Sort(keys)
	return keys
}

// minShadowingKeys is the number of variables one can shadow another from.
const minShadowingKeys = 2

// dropShadowedEnvs removes from the sorted keys the variables that set the
// same value as another one (see envTarget) and lose to it: the winner has the
// fewest Go field name levels, then comes first in name order. A single
// variable shadows none, so its target is not resolved.
func (cf *Config) dropShadowedEnvs(keys []string) []string {
	if len(keys) < minShadowingKeys {
		return keys
	}
	type candidate struct {
		key  string
		rank int
	}
	var (
		winners map[string]candidate
		losers  map[string]struct{}
	)
	for _, envKey := range keys {
		target, rank, ok := cf.envTarget(envKey)
		if !ok {
			continue
		}
		if winners == nil {
			winners = make(map[string]candidate)
		}
		current, seen := winners[target]
		if !seen {
			winners[target] = candidate{key: envKey, rank: rank}
			continue
		}
		loser := envKey
		if rank < current.rank || rank == current.rank && envKey < current.key {
			loser = current.key
			winners[target] = candidate{key: envKey, rank: rank}
		}
		if losers == nil {
			losers = make(map[string]struct{})
		}
		losers[loser] = struct{}{}
	}
	if losers == nil {
		return keys
	}
	return slices.DeleteFunc(keys, func(key string) bool {
		_, lost := losers[key]
		return lost
	})
}

// preferredEnvName returns the name of names set in envs that wins, as on
// the other paths of the environment loader: the one with the fewest Go field
// name levels (see aliasLevels), then the first in name order.
func (cf *Config) preferredEnvName(names []string, envs map[string]string) (string, bool) {
	first := -1
	for i, name := range names {
		if _, ok := envs[name]; !ok {
			continue
		}
		if first >= 0 {
			return cf.preferredOf(names[first:], envs), true
		}
		first = i
	}
	if first < 0 {
		return "", false
	}
	return names[first], true
}

// preferredOf returns the name of names set in envs that wins, when several
// are set (see preferredEnvName).
func (cf *Config) preferredOf(names []string, envs map[string]string) string {
	var (
		best     string
		bestRank int
	)
	for _, name := range names {
		if _, ok := envs[name]; !ok {
			continue
		}
		_, rank, _ := cf.envTarget(name)
		if best == "" || rank < bestRank || rank == bestRank && name < best {
			best, bestRank = name, rank
		}
	}
	return best
}

// envVarError names the environment variable name, without the prefix, in
// err. The value is never quoted: it may be an expanded secret.
func (cf *Config) envVarError(name string, err error) error {
	if err == nil {
		return nil
	}
	return coreerrs.Wrapf(err, "environment variable %s%s", cf.options.envPrefix, name)
}

// fieldByIndexAlloc returns the field at the struct index path from v,
// allocating nil pointer structs along the way.
func fieldByIndexAlloc(v reflect.Value, index []int) reflect.Value {
	for _, i := range index {
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v
}

// noteEnvGrowth records, before the environment grows the slice v at steps
// to n elements or creates the nil map v there, how many entries it held, so
// default tags fill only the entries the environment added.
func (cf *Config) noteEnvGrowth(v reflect.Value, n int, steps []pathStep) {
	switch {
	case v.Kind() == reflect.Slice && v.Len() < n,
		v.Kind() == reflect.Map && v.IsNil():
		if cf.present == nil {
			cf.present = &presence{}
		}
		cf.present.markGrown(steps, v.Len())
	default:
	}
}

// markEnv records that the environment set the value at steps.
func (cf *Config) markEnv(steps []pathStep) {
	if cf.present == nil {
		cf.present = &presence{}
	}
	cf.present.mark(steps)
}

// loadArrayEnvs handles array element environment variables using configured
// delimiter, among the variables keys names.
func (cf *Config) loadArrayEnvs(envs map[string]string, keys []string) error {
	// Group environment variables by array pattern
	arrayEnvs := cf.parseArrayEnvs(envs, keys)

	// Sort array paths by depth (number of delimiters) to process parent arrays first
	arrayPaths := make([]string, 0, len(arrayEnvs))
	for fieldPath := range arrayEnvs {
		arrayPaths = append(arrayPaths, fieldPath)
	}

	// Sort by depth (fewer delimiters = shallower = process first), then by
	// name, so the order does not depend on map iteration.
	delimiter := cf.options.envSectionDelimiter
	slices.SortFunc(arrayPaths, func(a, b string) int {
		return cmp.Or(cmp.Compare(strings.Count(a, delimiter), strings.Count(b, delimiter)), strings.Compare(a, b))
	})

	// Process each array pattern in order of depth
	for _, fieldPath := range arrayPaths {
		if err := cf.setArrayElements(fieldPath, arrayEnvs[fieldPath]); err != nil {
			return err
		}
	}

	return nil
}

// parseArrayEnvs parses environment variables to find array patterns.
// Returns a map where key is the field path and value is a map
// of element paths to their values using configured section delimiter. It
// reads the variables keys names.
func (cf *Config) parseArrayEnvs(envs map[string]string, keys []string) map[string]map[string]string {
	arrayEnvs := make(map[string]map[string]string)

	for _, envKey := range keys {
		cf.processArrayEnvKey(envKey, envs[envKey], arrayEnvs)
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

	// A name that addresses nothing settable is left out, so its candidate
	// slices allocate nothing along its path.
	if !cf.indexedKeyResolves(envKey, parts) {
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

	// A field of the field list under a nil pointer section has no value
	// yet: allocate the sections along its index path, as the plain-field
	// pass does.
	if !arrayField.value.IsValid() && arrayField.index != nil {
		if arrayField.field.Type.Kind() != reflect.Slice {
			return nil
		}
		allocated := *arrayField
		allocated.value = fieldByIndexAlloc(reflect.ValueOf(cf.conf).Elem(), arrayField.index)
		arrayField = &allocated
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

	return cf.setSliceElements(arrayField, fieldPath, elements)
}

// matchesArrayField checks if a field matches the array field path from environment variable.
func (cf *Config) matchesArrayField(fld *field, fieldPath string) bool {
	return slices.Contains(cf.envFieldNames(fld), fieldPath)
}

// setSliceElements sets individual elements in a slice based on environment
// variables. slicePath names the slice in their names.
func (cf *Config) setSliceElements(arrayField *field, slicePath string, elements map[string]string) error {
	sliceType := arrayField.value.Type()
	elementType := sliceType.Elem()

	// Check if this is a slice of primitives or a slice of structs
	isPrimitiveSlice := elementType.Kind() != reflect.Struct &&
		(elementType.Kind() != reflect.Pointer || elementType.Elem().Kind() != reflect.Struct)

	if isPrimitiveSlice {
		// Handle slice of primitives (e.g., []string, []int)
		return cf.setPrimitiveSliceElements(arrayField, slicePath, elements)
	}

	// Handle slice of structs
	return cf.setStructSliceElements(arrayField, slicePath, elements)
}

// setPrimitiveSliceElements sets elements in a slice of primitive types.
func (cf *Config) setPrimitiveSliceElements(arrayField *field, slicePath string, elements map[string]string) error {
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

		if err := cf.checkEnvIndex(arrayField.fullName, index, arrayField.value.Len()); err != nil {
			return err
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
	steps := arrayField.pathSteps()
	for index, value := range indexedValues {
		elementValue := arrayField.value.Index(index)
		if err := cf.setFieldValue(elementValue, value, withStep(steps, pathStep{kind: elemStep, index: index})); err != nil {
			return cf.envVarError(slicePath+cf.options.envSectionDelimiter+strconv.Itoa(index), err)
		}
	}

	return nil
}

// setStructSliceElements sets elements in a slice of struct types.
func (cf *Config) setStructSliceElements(arrayField *field, slicePath string, elements map[string]string) error {
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
		if err := cf.checkEnvIndex(arrayField.fullName, index, arrayField.value.Len()); err != nil {
			return err
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
	steps := arrayField.pathSteps()
	for index, fieldValues := range indexedElements {
		elementValue := arrayField.value.Index(index)
		elementSteps := withStep(steps, pathStep{kind: elemStep, index: index})
		elementName := slicePath + delimiter + strconv.Itoa(index)
		if err := cf.setStructFields(elementValue, elementName, fieldValues, elementSteps); err != nil {
			return err
		}
	}

	return nil
}

// checkEnvIndex validates a slice index read from an environment variable
// name for the slice name of length n: the index must not be negative, and
// an index past the slice must stay below the maxEnvSliceLen limit, so the
// slice never grows past it (and index+1 cannot overflow).
func (cf *Config) checkEnvIndex(name string, index, n int) error {
	if index >= 0 && (index < n || index < cf.options.maxEnvSliceLen) {
		return nil
	}
	return fmt.Errorf("%w: index %d of %s (length %d, limit %d)", ErrEnvIndexOutOfRange, index, name, n, cf.options.maxEnvSliceLen)
}

// ensureSliceSize ensures that a field's slice has at least the specified size.
// It delegates to ensureSliceSizeForValue and stores map entries back if needed.
func (cf *Config) ensureSliceSize(arrayField *field, minSize int) error {
	cf.noteEnvGrowth(arrayField.value, minSize, arrayField.pathSteps())

	// Use the base function to resize the slice
	if err := cf.ensureSliceSizeForValue(arrayField.value, minSize); err != nil {
		return err
	}

	// A slice inside map entries held by value was resized in copies of
	// them: store the copies back. Elements set afterwards share the backing
	// array of the stored slice.
	arrayField.storeEntries()

	return nil
}

// setStructFields sets fields of a struct based on a map of field names to values.
// Supports nested field paths using configured delimiter. name names
// structValue in the variable names; steps locates it from the configuration
// root. The paths are applied in name order.
func (cf *Config) setStructFields(structValue reflect.Value, name string, fieldValues map[string]string, steps []pathStep) error {
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
	for _, fieldPath := range slices.Sorted(maps.Keys(fieldValues)) {
		parts := strings.Split(fieldPath, delimiter)
		if err := cf.setNestedStructField(structValue, parts, fieldValues[fieldPath], steps); err != nil {
			return cf.envVarError(name+delimiter+fieldPath, err)
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
// It records the value as set by the environment only when it was actually
// assigned: an empty value leaves numbers, bools, slices and maps untouched,
// so their defaults still apply. steps locates fieldValue from the
// configuration root.
func (cf *Config) setFieldValue(fieldValue reflect.Value, value string, steps []pathStep) error {
	assigned, err := set(fieldValue, value, false, true, cf.options.strict)
	if err != nil {
		return err
	}
	if assigned {
		cf.markEnv(steps)
	}
	return nil
}

// loadNestedEnvs handles nested structures using the section delimiter. It
// applies the variables the sorted keys name, in that order.
func (cf *Config) loadNestedEnvs(envs map[string]string, keys []string) error {
	for _, envKey := range keys {
		if err := cf.setNestedFieldFromEnv(envKey, envs[envKey]); err != nil {
			return cf.envVarError(envKey, err)
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

	// A slice or map field with an env tag is addressed through the tag:
	// TAG__<index|key>. The tag claims the variable only for a valid entry
	// path, so nothing is allocated for a variable that sets nothing.
	if tagged, entryPath := cf.findTaggedCollection(envKey, false); tagged != nil {
		if entryPath == nil {
			// The tag names the whole variable: the plain-field pass set it.
			return nil
		}
		if _, ok := cf.settablePath(tagged.field.Type, entryPath, false); !ok {
			// A slice entry path, which the indexed pass sets.
			return nil
		}
		if !tagged.value.IsValid() {
			allocated := *tagged
			allocated.value = fieldByIndexAlloc(reflect.ValueOf(cf.conf).Elem(), tagged.index)
			tagged = &allocated
		}
		return cf.setNestedFieldValue(tagged, entryPath, envValue, tagged.pathSteps(), false)
	}

	// Find the root field
	rootFieldName := parts[0]
	var rootField *field

	cf.fields.each(func(fld *field) bool {
		// Check if this field matches the root field name
		// Only match root-level fields (no parent)
		if fld.parent == nil && cf.fieldNames(fld).matches(rootFieldName, true) {
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

	// Resolve the rest of the path on the type first, so a variable that sets
	// nothing allocates no nil section or map along it.
	if missing, ok := cf.settablePath(rootField.field.Type, parts[1:], true); !ok {
		if missing != "" && cf.options.strict {
			return fmt.Errorf("%w: %s", ErrFieldNotFound, missing)
		}
		return nil
	}

	// Navigate to the nested field and set its value
	return cf.setNestedFieldValue(rootField, parts[1:], envValue, rootField.pathSteps(), true)
}

// findRootFieldByName finds a root-level field in the config struct by name.
// This is used to find fields with default:"-" that haven't been initialized
// yet. A nil pointer field is left nil: the navigation allocates it only for a
// path that resolves (see settablePath).
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
	names := cf.structEnvNames(configType)
	for i := range names {
		if names[i].matches(fieldName, true) {
			structField := configType.Field(i)
			fieldValue := configValue.Field(i)

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
				index:    []int{i},
			}
		}
	}

	return nil
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// This file contains the struct parser for MongoDB document conversion.
// It separates metadata collection from business logic processing.

package mongo

import (
	"cmp"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// Constants for better code readability
const (
	// AlgorithmIndex is the position of the algorithm name in an encryption tag value.
	AlgorithmIndex = 0
	// KeyAltNameIndex is the position of the key alternative name in an encryption tag value.
	KeyAltNameIndex = 1
	// MinEncryptionTagParts is the minimum number of comma-separated parts
	// required in an encryption tag for it to be valid.
	MinEncryptionTagParts = 2

	// DefaultCacheEvictionAge is the default time after which cached entries are evicted
	DefaultCacheEvictionAge = 10 * time.Minute

	// Memory optimization constants
	CapacityBufferMultiplier = 1.25 // Multiplier for capacity estimation buffer
	LargeStructThreshold     = 100  // Threshold for large structs

	// Advanced caching constants
	MaxCacheSize           = 1000 // Maximum number of cached types
	CacheEvictionBatchSize = 50   // Number of entries to evict when cache is full

	// Sharding constants for high-concurrency scenarios
	DefaultNumShards = 16 // Number of cache shards (must be power of 2)

	// EvictionLowWaterNumerator and EvictionLowWaterDenominator express
	// the low-water mark as a 3/4 (75%) fraction of shard capacity.
	// The shard evicts down to this mark on overflow so saturated
	// shards strictly shrink instead of treadmilling at capacity.
	EvictionLowWaterNumerator   = 3
	EvictionLowWaterDenominator = 4
)

// CachedStructMetadata contains metadata with minimal overhead
type CachedStructMetadata struct {
	metadata     *StructMetadata
	lastAccessed atomic.Int64 // Unix timestamp of last access
	creationTime time.Time    // Time when this cache entry was created
}

// parserCacheShard is a single shard of the parser cache.
// Each shard has its own mutex to reduce contention in high-concurrency scenarios.
type parserCacheShard struct {
	entries     map[reflect.Type]*CachedStructMetadata
	mu          sync.RWMutex
	maxSize     int
	evictionAge time.Duration
}

// ParserCache provides sharded caching for parser metadata.
// Sharding reduces mutex contention by distributing types across multiple
// independent shards, each with its own lock.
//
// With 16 shards (default), concurrent access to different types will
// likely hit different shards, allowing parallel reads and writes.
type ParserCache struct {
	shards    []*parserCacheShard
	numShards uint64
}

// newParserCache creates a new sharded parser cache.
// The maxSize is distributed across all shards.
func newParserCache(maxSize int) *ParserCache {
	numShards := uint64(DefaultNumShards)
	shardMaxSize := max(1, maxSize/DefaultNumShards)

	shards := make([]*parserCacheShard, numShards)
	for i := range shards {
		shards[i] = &parserCacheShard{
			entries:     make(map[reflect.Type]*CachedStructMetadata),
			maxSize:     shardMaxSize,
			evictionAge: DefaultCacheEvictionAge,
		}
	}

	return &ParserCache{
		shards:    shards,
		numShards: numShards,
	}
}

// getShard returns the shard for a given type using FNV-1a hash.
func (pc *ParserCache) getShard(t reflect.Type) *parserCacheShard {
	// Use type's string representation for hashing
	// This is computed once per type access which is acceptable
	h := fnvHash(t.String())
	return pc.shards[h&(pc.numShards-1)] // Fast modulo for power of 2
}

// fnvHash computes FNV-1a hash of a string.
// This is a fast, well-distributed hash suitable for sharding.
func fnvHash(s string) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)

	h := uint64(offset64)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return h
}

// Get retrieves cached metadata with minimal overhead.
// Sharding ensures that concurrent gets for different types
// don't contend on the same lock.
func (pc *ParserCache) Get(t reflect.Type) (*StructMetadata, bool) {
	shard := pc.getShard(t)

	shard.mu.RLock()
	cached, exists := shard.entries[t]
	if !exists {
		shard.mu.RUnlock()
		return nil, false
	}

	// Update access time atomically
	cached.lastAccessed.Store(time.Now().Unix())

	metadata := cached.metadata
	shard.mu.RUnlock()

	return metadata, true
}

// Put stores metadata in cache with minimal overhead.
// Sharding ensures that concurrent puts for different types
// don't contend on the same lock.
func (pc *ParserCache) Put(t reflect.Type, metadata *StructMetadata) {
	shard := pc.getShard(t)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	now := time.Now()

	// Check if entry already exists - don't overwrite, just update access time
	if existing, exists := shard.entries[t]; exists {
		existing.lastAccessed.Store(now.Unix())
		return
	}

	// Check if we need to evict entries
	if len(shard.entries) >= shard.maxSize {
		shard.evictLRUEntries()
	}

	cached := &CachedStructMetadata{
		metadata:     metadata,
		creationTime: now,
	}
	cached.lastAccessed.Store(now.Unix())

	shard.entries[t] = cached
}

// SetEvictionAge sets the eviction age for all shards.
// This should be called during initialization, not concurrently with cache operations.
func (pc *ParserCache) SetEvictionAge(age time.Duration) {
	for _, shard := range pc.shards {
		shard.evictionAge = age
	}
}

// Len returns the total number of entries across all shards.
// This is an approximate count as shards are not locked together.
func (pc *ParserCache) Len() int {
	total := 0
	for _, shard := range pc.shards {
		shard.mu.RLock()
		total += len(shard.entries)
		shard.mu.RUnlock()
	}
	return total
}

// Clear removes all entries from all shards.
// This is useful for testing or memory cleanup.
func (pc *ParserCache) Clear() {
	for _, shard := range pc.shards {
		shard.mu.Lock()
		clear(shard.entries)
		shard.mu.Unlock()
	}
}

// evictLRUEntries removes least recently used entries when shard is full.
func (shard *parserCacheShard) evictLRUEntries() {
	if len(shard.entries) < shard.maxSize {
		return
	}

	// Collect eviction candidates
	type evictionCandidate struct {
		t            reflect.Type
		cached       *CachedStructMetadata
		lastAccessed int64
		age          time.Duration
	}

	candidates := make([]evictionCandidate, 0, len(shard.entries))
	now := time.Now()

	for t, cached := range shard.entries {
		candidates = append(candidates, evictionCandidate{
			t:            t,
			cached:       cached,
			lastAccessed: cached.lastAccessed.Load(),
			age:          now.Sub(cached.creationTime),
		})
	}

	// Sort by priority (old lastAccessed, old age = high priority for eviction)
	slices.SortFunc(candidates, func(a, b evictionCandidate) int {
		// First: prefer older entries by last access
		if a.lastAccessed != b.lastAccessed {
			return int(a.lastAccessed - b.lastAccessed)
		}

		// Second: prefer entries created longer ago
		return int(a.age - b.age)
	})

	// Evict the oldest/least used entries until the shard is below the
	// low-water mark. Previously this was a fixed batch of 3 entries
	// per shard (CacheEvictionBatchSize / DefaultNumShards = 50/16 ≈ 3)
	// which meant a saturated shard stayed saturated forever — every
	// eviction removed 3 entries only to have 3 new misses repopulate
	// them. Targeting a low-water mark (75% of shard capacity)
	// guarantees the cache actually shrinks on overflow and keeps the
	// active set fresh.
	lowWater := max(1, shard.maxSize*EvictionLowWaterNumerator/EvictionLowWaterDenominator)
	target := len(shard.entries) - lowWater
	if target <= 0 {
		return
	}
	evicted := 0
	for _, candidate := range candidates {
		if evicted >= target {
			break
		}

		// Only evict if entry is old enough
		if candidate.age > shard.evictionAge {
			delete(shard.entries, candidate.t)
			evicted++
		}
	}
}

// tagParser handles BSON tag parsing logic
type tagParser struct{}

// parseBSONTag extracts field information from BSON tags
func (tp *tagParser) parseBSONTag(tag string) (fieldName string, omitEmpty, omitOnUpdate bool) {
	parts := strings.Split(tag, ",")
	if len(parts) == 0 {
		return "", false, false
	}

	// Extract field name
	fieldName = parts[0]
	if fieldName == "" || fieldName == "-" {
		fieldName = ""
	}

	// Check for modifiers
	omitEmpty = slices.Contains(parts, "omitempty")
	omitOnUpdate = slices.Contains(parts, "omitonupdate")

	return
}

// parseEncryptionTag extracts encryption information from tags
func (tp *tagParser) parseEncryptionTag(tag string) (algorithm, keyAltName string, shouldEncrypt bool) {
	parts := strings.Split(tag, ",")
	if len(parts) < MinEncryptionTagParts || parts[AlgorithmIndex] == "" || parts[KeyAltNameIndex] == "" {
		return "", "", false
	}

	return parts[AlgorithmIndex], parts[KeyAltNameIndex], true
}

// nestedStructureDetector handles detection of nested structures
type nestedStructureDetector struct{}

// detectNestedType determines the type of nested structure
func (nsd *nestedStructureDetector) detectNestedType(fieldValue reflect.Value) NestedStructType {
	switch {
	case nsd.isPointerToStruct(fieldValue):
		return PointerStruct
	case nsd.isSliceOfStructs(fieldValue):
		return SliceStruct
	case nsd.isMapWithStructValues(fieldValue):
		return MapStruct
	default:
		return NoNesting
	}
}

// isPointerToStruct checks if the field type is a pointer to a struct.
func (nsd *nestedStructureDetector) isPointerToStruct(fieldValue reflect.Value) bool {
	return fieldValue.Kind() == reflect.Pointer &&
		fieldValue.Type().Elem().Kind() == reflect.Struct
}

// isSliceOfStructs checks if the field type is a slice/array of structs.
func (nsd *nestedStructureDetector) isSliceOfStructs(fieldValue reflect.Value) bool {
	if fieldValue.Kind() != reflect.Slice && fieldValue.Kind() != reflect.Array {
		return false
	}

	elemType := getElementType(fieldValue)
	return elemType.Kind() == reflect.Struct
}

// isMapWithStructValues checks if the field type is a map with struct values.
func (nsd *nestedStructureDetector) isMapWithStructValues(fieldValue reflect.Value) bool {
	if fieldValue.Kind() != reflect.Map {
		return false
	}

	elemType := getElementType(fieldValue)
	return elemType.Kind() == reflect.Struct
}

// fieldProcessor processes individual fields and creates metadata
type fieldProcessor struct {
	parser         *Parser
	tagParser      *tagParser
	nestedDetector *nestedStructureDetector
}

// newFieldProcessor creates a new field processor
func newFieldProcessor(parser *Parser) *fieldProcessor {
	return &fieldProcessor{
		parser:         parser,
		tagParser:      &tagParser{},
		nestedDetector: &nestedStructureDetector{},
	}
}

// processField creates metadata for a single field
func (fp *fieldProcessor) processField(fieldType reflect.StructField, fieldValue reflect.Value, parentType reflect.Type) *fieldMetadata {
	// Extract field name from BSON tag
	fieldName, omitEmpty, omitOnUpdate := fp.tagParser.parseBSONTag(fieldType.Tag.Get(fp.parser.bsonTagName))
	fieldName = cmp.Or(fieldName, corestrings.InternLowerString(fieldType.Name))

	// Skip fields with "-" or empty names after processing
	if fieldName == "" {
		return nil
	}

	// Use pooled fieldMetadata to reduce allocations
	meta := getFieldMetadata()

	// Fill metadata directly without builder overhead
	meta.fieldName = fieldName
	meta.fieldValue = fieldValue
	meta.fieldKind = fieldValue.Kind()
	meta.fieldType = fieldType
	meta.fieldIndex = fieldType.Index
	meta.isOmitEmpty = omitEmpty
	meta.isOmitOnUpdate = omitOnUpdate

	// Add encryption information
	algorithm, keyAltName, shouldEncrypt := fp.getEncryptionInfo(parentType, fieldType)
	meta.algorithmString = algorithm
	meta.keyAltName = keyAltName
	meta.shouldEncrypt = shouldEncrypt

	// Detect nested structures
	meta.nestedType = fp.nestedDetector.detectNestedType(fieldValue)

	return meta
}

// getEncryptionInfo retrieves encryption information for a field
func (fp *fieldProcessor) getEncryptionInfo(entityType reflect.Type, fieldType reflect.StructField) (string, string, bool) {
	// First, try to get encryption info from configured models
	if model, exists := fp.parser.encryptionModels[entityType]; exists {
		for _, field := range model.Fields {
			if field.FieldName == fieldType.Name {
				return field.Algorithm, field.KeyAltName, true
			}
		}
	}

	// Fallback to struct tags
	return fp.tagParser.parseEncryptionTag(fieldType.Tag.Get(fp.parser.encryptionTagName))
}

// embeddedStructProcessor handles embedded struct processing
type embeddedStructProcessor struct {
	fieldProcessor *fieldProcessor
}

// processEmbeddedStruct processes embedded structs recursively
func (esp *embeddedStructProcessor) processEmbeddedStruct(
	fieldType reflect.StructField, fieldValue reflect.Value, parentType reflect.Type,
) []fieldMetadata {
	// Handle pointer to embedded struct
	if fieldValue.Kind() == reflect.Pointer {
		if fieldValue.IsNil() {
			return nil
		}
		fieldValue = fieldValue.Elem()
	}

	// Only process struct types
	embeddedType := indirectType(fieldType.Type)
	if embeddedType.Kind() != reflect.Struct {
		return nil
	}

	var embeddedFields []fieldMetadata

	// Process each field in the embedded struct
	for i := range embeddedType.NumField() {
		embeddedFieldType := embeddedType.Field(i)
		embeddedFieldValue := fieldValue.Field(i)

		// Skip unexported fields
		if embeddedFieldType.PkgPath != "" {
			continue
		}

		// Handle nested embedded structs recursively
		if embeddedFieldType.Anonymous {
			nestedFields := esp.processEmbeddedStruct(embeddedFieldType, embeddedFieldValue, parentType)
			embeddedFields = append(embeddedFields, nestedFields...)
			continue
		}

		// Process regular field
		if meta := esp.fieldProcessor.processField(embeddedFieldType, embeddedFieldValue, parentType); meta != nil {
			// For embedded fields, we don't process deep nesting to avoid complexity
			meta.nestedType = esp.fieldProcessor.nestedDetector.detectNestedType(embeddedFieldValue)
			meta.hasNestedData = false
			// processField sets fieldIndex relative to the embedded struct; override with
			// the full path from parentType so FieldByIndex works correctly on cache hits.
			if sf, ok := parentType.FieldByName(embeddedFieldType.Name); ok {
				meta.fieldIndex = sf.Index
			}
			embeddedFields = append(embeddedFields, *meta)
		}
	}

	return embeddedFields
}

// StructMetadata contains complete metadata information for a parsed structure.
// This includes the struct's own fields and any nested structure information.
type StructMetadata struct {
	// StructType is the reflect.Type of the parsed struct
	StructType reflect.Type
	// Fields contains metadata for all direct fields of the struct
	Fields []fieldMetadata
	// NestedStructs contains metadata for nested structures found during parsing
	NestedStructs map[reflect.Type]*StructMetadata
}

// Parser handles struct field parsing and metadata collection for MongoDB document conversion.
// It provides separation between metadata collection and business logic processing.
// All methods are thread-safe and can be used concurrently with advanced caching.
type Parser struct {
	// Configuration
	encryptionModels map[reflect.Type]*EncryptionModel

	// Tag names
	bsonTagName       string
	encryptionTagName string

	// Advanced caching system - thread-safe internally
	cache *ParserCache

	// Options
	update        bool
	shouldEncrypt bool
}

// NewParser creates a new Parser instance with the specified options.
// The parser is responsible for extracting metadata from struct fields and nested structures.
// Uses advanced caching by default for optimal performance.
func NewParser(opts ...ParserOption) *Parser {
	o := newParserOptions(opts...)

	cache := newParserCache(o.cacheSize)
	if o.cacheEvictionAge > 0 {
		cache.SetEvictionAge(o.cacheEvictionAge)
	}

	return &Parser{
		encryptionModels:  o.encryptionModels,
		bsonTagName:       o.bsonTagName,
		encryptionTagName: o.encryptionTagName,
		cache:             cache,
		update:            o.update,
		shouldEncrypt:     o.shouldEncrypt,
	}
}

// ParseStruct parses a Go struct with advanced caching and reference counting.
// This method is thread-safe and can be called concurrently with optimal performance.
func (p *Parser) ParseStruct(entity any) *StructMetadata {
	entityType := indirectType(reflect.TypeOf(entity))
	entityValue := reflect.Indirect(reflect.ValueOf(entity))

	// Fast path: structural metadata is cached per-type, but field values are
	// instance-specific and must be refreshed from the current entity on every call
	if cached, hit := p.cache.Get(entityType); hit {
		return p.refreshFieldValues(cached, entityValue)
	}

	// Slow path: need to parse - no external locking needed (cache handles it)
	// Initialize metadata with smart capacity estimation
	estimatedCapacity := p.estimateFieldCapacity(entityType)
	structMeta := &StructMetadata{
		StructType:    entityType,
		Fields:        make([]fieldMetadata, 0, estimatedCapacity),
		NestedStructs: make(map[reflect.Type]*StructMetadata),
	}

	// Get pooled processors to reduce allocations
	fieldProcessor := getFieldProcessor(p)
	defer putFieldProcessor(fieldProcessor)

	embeddedProcessor := &embeddedStructProcessor{fieldProcessor: fieldProcessor}

	// For large structs, use chunk processing to reduce peak memory
	fieldCount := entityType.NumField()
	var fieldsMetadata []fieldMetadata
	if fieldCount > LargeStructThreshold {
		fieldsMetadata = p.processLargeStruct(entityType, entityValue, fieldProcessor, embeddedProcessor, structMeta)
	} else {
		fieldsMetadata = p.processRegularStruct(entityType, entityValue, fieldProcessor, embeddedProcessor, structMeta)
	}

	structMeta.Fields = fieldsMetadata

	// Store in advanced cache with reference counting
	p.cache.Put(entityType, structMeta)

	return structMeta
}

// ClearCache clears all cached metadata, useful for testing or memory cleanup
func (p *Parser) ClearCache() {
	p.cache.Clear()
}

// refreshFieldValues returns a new StructMetadata by copying cached structural metadata
// and rebinding all instance-specific field values from entityValue.
func (p *Parser) refreshFieldValues(cached *StructMetadata, entityValue reflect.Value) *StructMetadata {
	result := &StructMetadata{
		StructType:    cached.StructType,
		Fields:        make([]fieldMetadata, len(cached.Fields)),
		NestedStructs: cached.NestedStructs,
	}

	for i, meta := range cached.Fields {
		result.Fields[i] = meta
		result.Fields[i].fieldValue = entityValue.FieldByIndex(meta.fieldIndex)
		if meta.hasNestedData {
			result.Fields[i].nestedMetadata = nil
			result.Fields[i].hasNestedData = false
		}
	}

	return result
}

// estimateFieldCapacity estimates the total number of fields including embedded structs
func (p *Parser) estimateFieldCapacity(entityType reflect.Type) int {
	capacity := 0
	for i := range entityType.NumField() {
		field := entityType.Field(i)
		if field.PkgPath != "" {
			continue // Skip unexported fields
		}

		if field.Anonymous {
			// For embedded structs, recursively estimate capacity
			embeddedType := indirectType(field.Type)
			if embeddedType.Kind() == reflect.Struct {
				capacity += p.estimateFieldCapacity(embeddedType)
			}
		} else {
			capacity++
		}
	}
	// Add buffer to avoid edge case reallocations
	return int(float64(capacity) * CapacityBufferMultiplier)
}

func (p *Parser) processStructField(
	entityType reflect.Type,
	entityValue reflect.Value,
	fieldIndex int,
	fieldProcessor *fieldProcessor,
	embeddedProcessor *embeddedStructProcessor,
	structMeta *StructMetadata,
	fieldsMetadata *[]fieldMetadata,
) {
	fieldType := entityType.Field(fieldIndex)
	fieldValue := entityValue.Field(fieldIndex)

	// Skip unexported fields
	if fieldType.PkgPath != "" {
		return
	}

	// Handle embedded structs
	if fieldType.Anonymous {
		embeddedFields := embeddedProcessor.processEmbeddedStruct(fieldType, fieldValue, entityType)
		*fieldsMetadata = append(*fieldsMetadata, embeddedFields...)
		return
	}

	// Process regular field
	if meta := fieldProcessor.processField(fieldType, fieldValue, entityType); meta != nil {
		// Handle nested structures for non-embedded fields
		p.processNestedStructures(meta, structMeta)
		*fieldsMetadata = append(*fieldsMetadata, *meta)
		// Return pooled fieldMetadata immediately after use
		putFieldMetadata(meta)
	}
}

// processRegularStruct handles normal-sized structs with optimized memory allocation
func (p *Parser) processRegularStruct(
	entityType reflect.Type,
	entityValue reflect.Value,
	fieldProcessor *fieldProcessor,
	embeddedProcessor *embeddedStructProcessor,
	structMeta *StructMetadata,
) []fieldMetadata {
	// Use smart capacity estimation to avoid reallocations
	estimatedCapacity := p.estimateFieldCapacity(entityType)
	fieldsMetadata := make([]fieldMetadata, 0, estimatedCapacity)

	fieldCount := entityType.NumField()
	for i := range fieldCount {
		p.processStructField(entityType, entityValue, i, fieldProcessor, embeddedProcessor, structMeta, &fieldsMetadata)
	}

	return fieldsMetadata
}

// processLargeStruct handles large structs (>100 fields) with chunk processing to reduce peak memory
func (p *Parser) processLargeStruct(
	entityType reflect.Type,
	entityValue reflect.Value,
	fieldProcessor *fieldProcessor,
	embeddedProcessor *embeddedStructProcessor,
	structMeta *StructMetadata,
) []fieldMetadata {
	fieldCount := entityType.NumField()
	// Use smart capacity estimation even for large structs to avoid reallocations
	estimatedCapacity := p.estimateFieldCapacity(entityType)
	fieldsMetadata := make([]fieldMetadata, 0, estimatedCapacity)

	// Process in chunks of 50 to limit memory spikes
	const chunkSize = 50

	for chunkStart := 0; chunkStart < fieldCount; chunkStart += chunkSize {
		chunkEnd := min(chunkStart+chunkSize, fieldCount)

		// Process chunk
		for i := chunkStart; i < chunkEnd; i++ {
			p.processStructField(entityType, entityValue, i, fieldProcessor, embeddedProcessor, structMeta, &fieldsMetadata)
		}
	}

	return fieldsMetadata
}

// processNestedStructures handles nested structure processing with improved clarity
func (p *Parser) processNestedStructures(meta *fieldMetadata, parentMeta *StructMetadata) {
	switch meta.nestedType {
	case NoNesting:
		// No nested structure processing needed
		return
	case PointerStruct:
		p.processPointerStruct(meta, parentMeta)
	case SliceStruct:
		p.processSliceStruct(meta, parentMeta)
	case MapStruct:
		p.processMapStruct(meta, parentMeta)
	}
}

// processPointerStruct processes pointer to struct with better error handling
func (p *Parser) processPointerStruct(meta *fieldMetadata, parentMeta *StructMetadata) {
	if meta.fieldValue.IsNil() {
		return
	}

	nestedValue := meta.fieldValue.Elem().Interface()
	nestedStructMeta := p.ParseStruct(nestedValue)

	meta.nestedMetadata = nestedStructMeta.Fields
	meta.hasNestedData = len(meta.nestedMetadata) > 0
	parentMeta.NestedStructs[nestedStructMeta.StructType] = nestedStructMeta
}

// processSliceStruct processes slice of structs with improved structure
func (p *Parser) processSliceStruct(meta *fieldMetadata, parentMeta *StructMetadata) {
	if meta.fieldValue.Len() == 0 {
		return
	}
	firstElem := p.getFirstNonNilElement(meta.fieldValue)
	p.processNestedStructValue(meta, parentMeta, firstElem)
}

// processMapStruct processes map with struct values
func (p *Parser) processMapStruct(meta *fieldMetadata, parentMeta *StructMetadata) {
	if meta.fieldValue.Len() == 0 {
		return
	}
	firstValue := p.getFirstMapValue(meta.fieldValue)
	p.processNestedStructValue(meta, parentMeta, firstValue)
}

// Helper functions for better code organization

// getFirstNonNilElement gets the first non-nil element from slice/array
func (p *Parser) getFirstNonNilElement(value reflect.Value) *reflect.Value {
	firstElem := value.Index(0)
	for firstElem.Kind() == reflect.Pointer {
		if firstElem.IsNil() {
			return nil
		}
		firstElem = firstElem.Elem()
	}
	return &firstElem
}

// getFirstMapValue gets the first value from a map
func (p *Parser) getFirstMapValue(value reflect.Value) *reflect.Value {
	mapIter := value.MapRange()
	if !mapIter.Next() {
		return nil
	}

	firstValue := mapIter.Value()
	for firstValue.Kind() == reflect.Pointer {
		if firstValue.IsNil() {
			return nil
		}
		firstValue = firstValue.Elem()
	}
	return &firstValue
}

func (p *Parser) processNestedStructValue(meta *fieldMetadata, parentMeta *StructMetadata, v *reflect.Value) {
	if v == nil || v.Kind() != reflect.Struct {
		return
	}

	nestedStructMeta := p.ParseStruct(v.Interface())
	meta.nestedMetadata = nestedStructMeta.Fields
	meta.hasNestedData = len(meta.nestedMetadata) > 0
	parentMeta.NestedStructs[nestedStructMeta.StructType] = nestedStructMeta
}

// getElementType returns the underlying element type, dereferencing pointer types
func getElementType(fieldValue reflect.Value) reflect.Type {
	elemType := fieldValue.Type().Elem()
	for elemType.Kind() == reflect.Pointer {
		elemType = elemType.Elem()
	}
	return elemType
}

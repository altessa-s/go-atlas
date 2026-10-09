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

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
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

// parseBSONTag extracts field information from BSON tags. skip reports the
// exclusion tag "-", which the driver treats the same way: the field is never
// encoded.
func (tp *tagParser) parseBSONTag(tag string) (fieldName string, omitEmpty, omitOnUpdate, skip bool) {
	if tag == "-" {
		return "", false, false, true
	}

	parts := strings.Split(tag, ",")
	if len(parts) == 0 {
		return "", false, false, false
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

// StructMetadata describes a struct and its fields. ParseStruct binds direct
// fields to the supplied instance; NestedStructs contains type descriptions
// only. Returned slices and maps do not alias the parser cache.
type StructMetadata struct {
	StructType    reflect.Type
	Fields        []fieldMetadata
	NestedStructs map[reflect.Type]*StructMetadata
}

// Parser caches type descriptions without retaining entity values. It is safe
// for concurrent use; callers own the metadata returned by ParseStruct.
type Parser struct {
	encryptionModels  *coremaps.ImmutableMap[reflect.Type, EncryptionModel]
	bsonTagName       string
	encryptionTagName string
	cache             *ParserCache
}

// NewParser creates a parser with a private snapshot of its encryption schema.
func NewParser(opts ...ParserOption) *Parser {
	o := newParserOptions(opts...)
	cache := newParserCache(o.cacheSize)
	if o.cacheEvictionAge > 0 {
		cache.SetEvictionAge(o.cacheEvictionAge)
	}
	models := make(map[reflect.Type]EncryptionModel, len(o.encryptionModels))
	for typ, model := range o.encryptionModels {
		if model != nil {
			cp := *model
			cp.Type = nil // the map key already identifies the type; do not retain a sample entity
			cp.Fields = slices.Clone(model.Fields)
			models[typ] = cp
		}
	}
	return &Parser{encryptionModels: coremaps.NewImmutableMap(models), bsonTagName: o.bsonTagName,
		encryptionTagName: o.encryptionTagName, cache: cache}
}

// ParseStruct returns fields bound to entity. Nil embedded pointers omit their
// promoted fields for this instance only. Invalid or nil inputs yield no fields.
func (p *Parser) ParseStruct(entity any) *StructMetadata {
	value := reflect.ValueOf(entity)
	for value.IsValid() && value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return &StructMetadata{}
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return &StructMetadata{}
	}
	typ := value.Type()
	layout, ok := p.cache.Get(typ)
	if !ok {
		layout = p.describeType(typ, make(map[reflect.Type]*StructMetadata))
		p.cache.Put(typ, layout)
	}
	result := &StructMetadata{StructType: typ, Fields: slices.Clone(layout.Fields)}
	if len(layout.NestedStructs) > 0 {
		result.NestedStructs = make(map[reflect.Type]*StructMetadata, len(layout.NestedStructs))
		copies := make(map[*StructMetadata]*StructMetadata)
		for nestedType, nested := range layout.NestedStructs {
			result.NestedStructs[nestedType] = copyMetadata(nested, copies)
		}
	}
	fields := result.Fields[:0]
	for _, field := range result.Fields {
		fieldValue, err := value.FieldByIndexErr(field.fieldIndex)
		if err != nil {
			continue
		} // nil embedded pointer in this particular entity
		field.fieldValue = fieldValue
		fields = append(fields, field)
	}
	result.Fields = fields
	return result
}

// ClearCache releases all cached type descriptions.
func (p *Parser) ClearCache() { p.cache.Clear() }

func (p *Parser) describeType(typ reflect.Type, seen map[reflect.Type]*StructMetadata) *StructMetadata {
	if meta, ok := seen[typ]; ok {
		return meta
	}
	meta := &StructMetadata{StructType: typ, NestedStructs: make(map[reflect.Type]*StructMetadata)}
	seen[typ] = meta
	meta.Fields = p.describeFields(typ, typ, nil, nil)
	for _, field := range meta.Fields {
		nested := nestedStructType(field.fieldType.Type)
		if nested != nil {
			meta.NestedStructs[nested] = p.describeType(nested, seen)
		}
	}
	return meta
}

func (p *Parser) describeFields(root, typ reflect.Type, prefix []int, ancestors []reflect.Type) []fieldMetadata {
	if slices.Contains(ancestors, typ) {
		return nil
	}
	ancestors = append(ancestors, typ)
	fields := make([]fieldMetadata, 0, typ.NumField())
	for sf := range typ.Fields() {
		if !sf.IsExported() {
			continue
		}
		name, omitEmpty, omitUpdate, skip := (&tagParser{}).parseBSONTag(sf.Tag.Get(p.bsonTagName))
		if skip {
			continue
		}
		index := append(slices.Clone(prefix), sf.Index...)
		if sf.Anonymous && indirectType(sf.Type).Kind() == reflect.Struct {
			fields = append(fields, p.describeFields(root, indirectType(sf.Type), index, ancestors)...)
			continue
		}
		algorithm, key, encrypt := p.encryptionInfo(root, sf)
		fields = append(fields, fieldMetadata{
			fieldType: sf, fieldIndex: index, fieldName: cmp.Or(name, corestrings.InternLowerString(sf.Name)),
			fieldKind: sf.Type.Kind(), isOmitEmpty: omitEmpty, isOmitOnUpdate: omitUpdate,
			algorithmString: algorithm, keyAltName: key, shouldEncrypt: encrypt, nestedType: nestingKind(sf.Type),
		})
	}
	return fields
}

func (p *Parser) encryptionInfo(typ reflect.Type, sf reflect.StructField) (string, string, bool) {
	if model, ok := p.encryptionModels.Get(typ); ok {
		for _, field := range model.Fields {
			if field.FieldName == sf.Name {
				return field.Algorithm, field.KeyAltName, true
			}
		}
	}
	return (&tagParser{}).parseEncryptionTag(sf.Tag.Get(p.encryptionTagName))
}

func nestedStructType(typ reflect.Type) reflect.Type {
	switch typ.Kind() { //nolint:exhaustive // Only container kinds can wrap nested structs.
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		elem := indirectType(typ.Elem())
		if elem.Kind() == reflect.Struct {
			return elem
		}
	}
	return nil
}

func nestingKind(typ reflect.Type) NestedStructType {
	if nestedStructType(typ) == nil {
		return NoNesting
	}
	switch typ.Kind() {
	case reflect.Pointer:
		return PointerStruct
	case reflect.Slice, reflect.Array:
		return SliceStruct
	case reflect.Map:
		return MapStruct
	default:
		return NoNesting
	}
}

func copyMetadata(src *StructMetadata, seen map[*StructMetadata]*StructMetadata) *StructMetadata {
	if previous, ok := seen[src]; ok {
		return previous
	}
	dst := &StructMetadata{StructType: src.StructType, Fields: slices.Clone(src.Fields)}
	seen[src] = dst
	if len(src.NestedStructs) > 0 {
		dst.NestedStructs = make(map[reflect.Type]*StructMetadata, len(src.NestedStructs))
		for typ, nested := range src.NestedStructs {
			dst.NestedStructs[typ] = copyMetadata(nested, seen)
		}
	}
	return dst
}

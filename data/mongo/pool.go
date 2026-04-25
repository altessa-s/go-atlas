// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"sync"
)

var (
	// fieldMetadataPool provides reusable fieldMetadata structs for heavy operations
	fieldMetadataPool = sync.Pool{
		New: func() any {
			return &fieldMetadata{}
		},
	}

	// fieldProcessorPool provides reusable fieldProcessor structs
	fieldProcessorPool = sync.Pool{
		New: func() any {
			return &fieldProcessor{
				tagParser:      &tagParser{},
				nestedDetector: &nestedStructureDetector{},
			}
		},
	}
)

// getFieldMetadata gets a clean fieldMetadata from the pool
func getFieldMetadata() *fieldMetadata {
	pooled := fieldMetadataPool.Get()
	if meta, ok := pooled.(*fieldMetadata); ok {
		// Reset all fields
		*meta = fieldMetadata{}
		return meta
	}
	// Fallback if type assertion fails
	return &fieldMetadata{}
}

// putFieldMetadata returns a fieldMetadata to the pool
func putFieldMetadata(meta *fieldMetadata) {
	if meta == nil {
		return
	}
	// Reset struct
	*meta = fieldMetadata{}
	fieldMetadataPool.Put(meta)
}

// getFieldProcessor gets a clean fieldProcessor from the pool
func getFieldProcessor(parser *Parser) *fieldProcessor {
	pooled := fieldProcessorPool.Get()
	if fp, ok := pooled.(*fieldProcessor); ok {
		// Set the parser reference
		fp.parser = parser
		return fp
	}
	// Fallback if type assertion fails
	return newFieldProcessor(parser)
}

// putFieldProcessor returns a fieldProcessor to the pool
func putFieldProcessor(fp *fieldProcessor) {
	if fp == nil {
		return
	}
	// Clear parser reference
	fp.parser = nil
	fieldProcessorPool.Put(fp)
}

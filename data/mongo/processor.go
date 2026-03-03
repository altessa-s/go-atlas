// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// This file contains field processing logic for MongoDB document conversion.
// It encapsulates the documentFieldProcessor and related processing strategies
// to handle different field types during BSON document conversion.

package mongo

import (
	"context"
	"fmt"
	"reflect"

	"go.mongodb.org/mongo-driver/v2/bson"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// fieldProcessingResult holds the result of field processing
type fieldProcessingResult struct {
	setDoc   bson.M
	unsetDoc bson.M
	skip     bool
}

// documentFieldProcessor encapsulates field processing logic with better separation of concerns
type documentFieldProcessor struct {
	mongo  *Mongo
	ctx    context.Context
	meta   fieldMetadata
	update bool
	result fieldProcessingResult
}

// newFieldProcessor creates a new field processor
func (m *Mongo) newFieldProcessor(ctx context.Context, meta fieldMetadata, update bool) *documentFieldProcessor {
	return &documentFieldProcessor{
		mongo:  m,
		ctx:    ctx,
		meta:   meta,
		update: update,
		result: fieldProcessingResult{
			setDoc:   make(bson.M),
			unsetDoc: make(bson.M),
			skip:     false,
		},
	}
}

// applyPreProcessingFilters applies filters that might skip field processing
func (fp *documentFieldProcessor) applyPreProcessingFilters() fieldProcessingResult {
	// Skip if field should be omitted on update
	if fp.update && fp.meta.isOmitOnUpdate {
		fp.result.skip = true
		return fp.result
	}

	// Handle nil pointer fields in update mode
	if fp.update && fp.meta.fieldValue.Kind() == reflect.Pointer && fp.meta.fieldValue.IsNil() {
		fp.result.unsetDoc[fp.meta.fieldName] = nil
		fp.result.skip = true
		return fp.result
	}

	// Handle omit empty fields in insert mode
	if !fp.update && fp.meta.isOmitEmpty && fp.meta.fieldValue.Kind() == reflect.Pointer &&
		(fp.meta.fieldValue.IsNil() || reflect.Indirect(fp.meta.fieldValue).IsZero()) {
		fp.result.skip = true
		return fp.result
	}

	// Handle empty collections
	if fp.mongo.isEmptyCollection(fp.meta.fieldValue) {
		if fp.update {
			fp.result.unsetDoc[fp.meta.fieldName] = nil
		}
		fp.result.skip = true
		return fp.result
	}

	return fp.result
}

// processFieldDirect handles field processing with direct method dispatch instead of Strategy pattern
func (fp *documentFieldProcessor) processFieldDirect() (bson.M, bson.M, error) {
	switch {
	case fp.meta.fieldKind == reflect.Bool:
		return fp.processBoolField()
	case fp.mongo.isSliceField(fp.meta.fieldValue, fp.meta.fieldType, fp.update) &&
		!fp.mongo.isBytesSlice(fp.meta.fieldValue):
		return fp.processSliceField()
	case fp.mongo.isStructPointerField(fp.meta.fieldValue, fp.meta.fieldType):
		return fp.processStructPointerField()
	case fp.mongo.isMapField(fp.meta.fieldValue, fp.meta.fieldType, fp.update):
		return fp.processMapField()
	default:
		return fp.processDefaultField()
	}
}

// Direct field processing methods (inline instead of strategies)

func (fp *documentFieldProcessor) processBoolField() (bson.M, bson.M, error) {
	fp.result.setDoc[fp.meta.fieldName] = fp.meta.fieldValue.Bool()
	return fp.result.setDoc, fp.result.unsetDoc, nil
}

func (fp *documentFieldProcessor) processSliceField() (bson.M, bson.M, error) {
	return fp.mongo.processSliceField(fp.ctx, fp.meta, fp.update,
		fp.result.setDoc, fp.result.unsetDoc)
}

func (fp *documentFieldProcessor) processStructPointerField() (bson.M, bson.M, error) {
	return fp.mongo.processStructPointerField(fp.ctx, fp.meta, fp.update,
		fp.result.setDoc, fp.result.unsetDoc)
}

func (fp *documentFieldProcessor) processMapField() (bson.M, bson.M, error) {
	return fp.mongo.processMapField(fp.ctx, fp.meta, fp.update,
		fp.result.setDoc, fp.result.unsetDoc)
}

func (fp *documentFieldProcessor) processDefaultField() (bson.M, bson.M, error) {
	return fp.mongo.processDefaultField(fp.ctx, fp.meta, fp.result.setDoc)
}

// documentProcessor encapsulates document conversion logic with clear separation of concerns
type documentProcessor struct {
	mongo *Mongo
}

// newDocumentProcessor creates a new document processor
func (m *Mongo) newDocumentProcessor() *documentProcessor {
	return &documentProcessor{mongo: m}
}

// convertEntityToDocuments converts an entity to MongoDB documents with proper error handling
func (dp *documentProcessor) convertEntityToDocuments(ctx context.Context, entity any, update, encrypt bool) (bson.M, bson.M, error) {
	// Step 1: Collect metadata
	fieldsMetadata, err := dp.collectFieldsMetadataWithValidation(entity, update, encrypt)
	if err != nil {
		return nil, nil, coreerrs.Wrapf(err, "document conversion failed during %s", "metadata collection")
	}

	// Step 2: Process fields
	setDoc, unsetDoc, err := dp.processFieldsWithErrorHandling(ctx, fieldsMetadata, update)

	// Explicit cleanup to avoid defer closure allocations
	dp.cleanupFieldMetadata(fieldsMetadata)

	if err != nil {
		return nil, nil, coreerrs.Wrapf(err, "document conversion failed during %s", "field processing")
	}

	return setDoc, unsetDoc, nil
}

// cleanupFieldMetadata returns fieldMetadata objects to pool without closure overhead
func (dp *documentProcessor) cleanupFieldMetadata(fieldsMetadata []fieldMetadata) {
	// Note: fieldsMetadata contains values, not pointers from pool
	// In our optimized parser, we return pooled objects immediately after use
	// This function is for future extensibility if needed
}

// collectFieldsMetadataWithValidation collects field metadata with validation
func (dp *documentProcessor) collectFieldsMetadataWithValidation(entity any, update, encrypt bool) ([]fieldMetadata, error) {
	if entity == nil {
		return nil, fmt.Errorf("entity cannot be nil")
	}

	fieldsMetadata := dp.mongo.collectFieldsMetadata(entity, update, encrypt)
	if len(fieldsMetadata) == 0 {
		return nil, fmt.Errorf("entity contains no processable fields")
	}

	return fieldsMetadata, nil
}

// processFieldsWithErrorHandling processes fields with consistent error handling.
// Shared set/unset maps are passed directly to each field processor,
// eliminating per-field map allocation and merge overhead.
func (dp *documentProcessor) processFieldsWithErrorHandling(ctx context.Context, fieldsMetadata []fieldMetadata, update bool) (bson.M, bson.M, error) {
	set := make(bson.M, len(fieldsMetadata))
	unset := make(bson.M)

	for _, meta := range fieldsMetadata {
		if err := dp.mongo.processFieldInto(ctx, meta, update, set, unset); err != nil {
			return nil, nil, coreerrs.WrapField(err, meta.fieldName)
		}
	}

	return set, unset, nil
}

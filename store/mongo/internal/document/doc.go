// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package document parses struct metadata and builds MongoDB documents.
// It does not own a MongoDB client; callers supply field encryption when needed.
//
// # Usage
//
//	engine := document.New(document.NewParser(), encryptField)
//	set, unset, err := engine.Convert(ctx, entity, true)
//
// # Ownership
//
// Parsers cache type descriptions without retaining entity values. Metadata
// returned by ParseStruct belongs to the caller. Nested updates replace whole
// subdocuments; their unset fields are omitted from the replacement value.
package document

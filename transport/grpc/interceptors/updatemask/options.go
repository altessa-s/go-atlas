// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package updatemask

//go:generate go tool optgen generate --type=options

import (
	"github.com/altessa-s/go-aip/fieldmask/extract"
	"github.com/altessa-s/go-aip/fieldmask/update"

	"github.com/altessa-s/go-atlas/internal/transport/updatevalidation"
)

// DefaultMaskFieldName is the update_mask field named in the errors of a
// [Normalizer] unless [WithMaskFieldName] renames it.
const DefaultMaskFieldName = "update_mask"

// options holds the configuration of a [Normalizer].
type options struct {
	// specLookup is installed by the internal registration adapter.
	specLookup updatevalidation.SpecLookup `opt:"-"`

	// WithExtractor sets how the update_mask and the resource of an Update
	// request are located. By default (DefaultExtractor), the singular
	// google.protobuf.FieldMask is declared as update_mask, falling back to
	// updateMask only when update_mask is not declared. Nil fn is ignored.
	extractor extract.UpdateFunc `optgen:"notnil"`

	// WithUpdateOptions adds options of update.Prepare: mask limits, path
	// validation, field behavior resolution. The Normalizer sets the update
	// mask field name, and strictness itself.
	updateOptions []update.Option `optgen:"append" optval:"clone"`

	// WithStrict refuses an Update request with no update_mask or with an
	// update_mask of no paths, and checks every path of the mask whatever the
	// path validation mode (update.WithStrict). A refusal names the mask field.
	strict bool `optgen:"default=false"`

	// WithMaskFieldName names the update_mask field in errors. Defaults to
	// DefaultMaskFieldName; lookup still selects the declared update_mask, or
	// updateMask only when update_mask is not declared. Use [WithExtractor]
	// to override lookup.
	maskFieldName string `optgen:"default=DefaultMaskFieldName"`
}

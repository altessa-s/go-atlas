// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=setFieldsOptions --option-type=SetFieldsOption --output=set_fields_options_gen.go

// setFieldsOptions tunes the paths [FromSetFields] returns. Configure it
// through the generated [SetFieldsOption] values.
type setFieldsOptions struct {
	// topLevelOnly keeps only the top-level fields of the message, without
	// the paths below them: the mask of fields an update would replace as a
	// whole.
	topLevelOnly bool
	// declarationOrder orders a []string result by the declaration order of
	// the top-level field each path starts with instead of
	// lexicographically; paths under one top-level field stay sorted. A
	// FieldMask result is unordered either way.
	declarationOrder bool
	// skipFields drops the named fields and everything below them; a name is
	// a path of declared field names, such as "id" or "profile.secret". A
	// nested name also drops the parents that would cover it (see
	// FromSetFields). Apply the resulting mask with Select or Merge, not
	// Filter: it is empty when every populated field is skipped.
	skipFields []string
}

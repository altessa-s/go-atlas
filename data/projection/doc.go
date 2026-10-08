// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package projection parses a client-supplied field list — the AIP-157
// partial-response "fields" parameter — and translates it into a
// database-specific projection, so unrequested fields are never read from
// storage instead of being trimmed off the response afterwards.
//
// It is the field-selection counterpart of data/orderby: a cached parser
// produces a [Spec], and a per-backend translator under translators/ turns it
// into a MongoDB projection document or a SQL column list, applying a shared
// policy of allowed, denied, mapped and required fields.
//
// # Grammar
//
//	fields := "" | "*" | path ("," path)*
//	path   := ident ("." ident)*
//	ident  := [A-Za-z_][A-Za-z0-9_]*
//
// This is a restricted subset of the AIP-161 field mask grammar, so a value
// from x-goog-fieldmask, $fields or fieldmaskpb.FieldMask.Paths parses as
// long as it stays inside the subset. Wildcard segments ("items.*.name"),
// backtick-quoted map keys and numeric segments ("items.0") are rejected
// with [ErrUnsupportedPath]. Empty, whitespace-only and "*" input select all
// authorized fields. Duplicates are dropped and the paths sorted.
//
// # Basic Usage
//
//	parser, _ := projection.NewParser()
//	spec, err := parser.Parse(ctx, req.GetFields()) // or parser.ParsePaths(ctx, mask.GetPaths())
//	if err != nil {
//	    return status.Error(codes.InvalidArgument, err.Error())
//	}
//
//	trans, err := mongo.NewTranslator(
//	    projection.WithUntrustedInput(),
//	    projection.WithAllowedFields("name", "createTime", "address.*"),
//	    projection.WithFieldMapping(map[string]string{"createTime": "created_at"}),
//	    projection.WithRequiredFields("_id"),
//	)
//	proj, err := trans.Translate(spec)
//
// # Policy
//
// The policy is evaluated per API path, then again on the storage name the
// mapping produces:
//
//   - WithAllowedFields restricts requestable paths ("address.*" allows the
//     subtree but not "address" itself).
//   - WithDeniedFields (API names) and WithDeniedStorageFields (storage
//     names) are never returned. A request overlapping a denied path — equal,
//     ancestor or descendant — fails, so selecting "credentials" cannot leak
//     "credentials.password".
//   - WithRequiredFields are added to every inclusion projection (IDs,
//     tenant keys, versions, keyset sort keys).
//   - WithDefaultFields sets what an empty request returns.
//
// # Empty requests
//
// "All fields" means all authorized fields, never a bypass of the policy.
// The default selection is derived once at construction: WithDefaultFields
// when set; otherwise the roots of a restrictive allow-list; otherwise —
// with no allow-list — every field except the denied ones, which MongoDB
// renders as an exclusion projection and SQL cannot express. Policies whose
// default cannot be derived safely fail at construction with
// [ErrDefaultFieldsRequired].
//
// # Security
//
// The parser caps input length ([WithMaxExpressionLength]), path count
// ([WithMaxPaths], before deduplication), path depth and path length.
// [WithUntrustedInput] makes a missing allow-list a construction error.
// Translators re-validate every path of a hand-built Spec, and a SQL column
// must be a plain identifier.
package projection

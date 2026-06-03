// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior

import "google.golang.org/genproto/googleapis/api/annotations"

// Violation describes a single field-behavior constraint that was violated
// during update-mask application or payload sanitization. It is the shared
// shape used by both [github.com/altessa-s/go-atlas/domain/proto/fieldmask]
// and [github.com/altessa-s/go-atlas/domain/proto/fieldbehavior] so the gRPC
// interceptor can render either source through the same google.rpc.BadRequest
// FieldViolation mapping.
//
// Reason is populated by callers that classify the violation with a
// human-readable explanation (fieldmask's required/immutable/identifier
// reasons). Behavior is populated by callers that surface the raw
// google.api.field_behavior annotation (fieldbehavior's strict-mode strip).
// Both fields are optional — only Path is guaranteed to be set.
type Violation struct {
	// Path is the dot-separated path to the offending field. Repeated and
	// map entries may be indexed by the producer ("aliases[2].id",
	// `labels["foo"].id`).
	Path string

	// Behavior is the google.api.field_behavior value that triggered the
	// violation. Zero (FIELD_BEHAVIOR_UNSPECIFIED) when the producer did not
	// classify the violation by annotation.
	Behavior annotations.FieldBehavior

	// Reason is a human-readable explanation of why the field was rejected.
	// Empty when the producer relies solely on Behavior to convey intent.
	Reason string
}

// JoinPath returns prefix joined with name using a dot separator. An empty
// prefix yields name unchanged so callers can build paths incrementally
// without special-casing the root.
func JoinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

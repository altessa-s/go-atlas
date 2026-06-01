// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package behavior reads the google.api.field_behavior annotation from a
// [protoreflect.FieldDescriptor].
//
// The annotation is defined by AIP-203 and surfaced through
// [annotations.E_FieldBehavior]. Multiple behaviors can be applied to a single
// field (for example REQUIRED+IMMUTABLE), so [Get] returns the full slice and
// callers decide which combination matters.
//
// This package is internal to domain/proto and shared between fieldmask and
// fieldbehavior. The two consumers interpret the behaviors differently —
// fieldmask collapses them into an enum that governs update-mask validation,
// fieldbehavior treats them as a set used to strip request/response payloads —
// so the helper deliberately stops at extraction.
package behavior

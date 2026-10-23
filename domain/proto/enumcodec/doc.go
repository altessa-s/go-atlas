// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package enumcodec maps a generated proto enum to a domain enum and back.
//
// [New] builds a bidirectional [Codec] from a static one-to-one table of the
// defined proto values. The proto zero value (UNSPECIFIED) decodes to the
// zero domain value, so a partial update can leave an enum unset and domain
// validation decides where a value is required; an undefined proto value is
// a core/errors.FieldError on the API field path ("unknown value 7"), which
// the gRPC errstatus canonical rules report as a BadRequest violation.
// Encoding an unknown or zero domain value with [Codec.Proto] yields UNSPECIFIED.
// [Codec.ProtoE] rejects unknown non-zero domain values with a FieldError;
// zero still encodes as UNSPECIFIED. [Codec.ErrorCodec] adapts both directions
// to converter.WithErrorCodecs, preserving full source paths on errors. It handles
// exact P/D and *P/*D pairs, including proto3 optional enums. Non-nil pointers
// produce newly allocated destination pointers; nil pointers passed to the adapter
// clear the destination. Struct conversion still skips nil source pointer fields.
//
//	var gradeCodec = enumcodec.New(map[refv1.Grade]entities.Grade{
//		refv1.Grade_GRADE_JUNIOR: entities.GradeJunior,
//		refv1.Grade_GRADE_SENIOR: entities.GradeSenior,
//	})
//
//	grade, err := gradeCodec.Domain("referenceItem.roleGrade.grade", p.GetGrade())
//	p.Grade = gradeCodec.Proto(item.Grade)
package enumcodec

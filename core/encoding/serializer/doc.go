// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package serializer provides a common [Serializer] interface for encoding and
// decoding arbitrary Go values. It is used by cache, uniq, and other packages
// that need format-agnostic value marshaling.
//
// The default implementation is [JSON], which delegates to [encoding/json].
//
// Example:
//
//	s := &serializer.JSON{}
//	data, _ := s.Serialize(myStruct)
//	_ = s.Deserialize(data, &result)
package serializer

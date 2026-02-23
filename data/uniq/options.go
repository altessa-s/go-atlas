// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uniq

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"github.com/altessa-s/go-atlas/core/encoding/serializer"
)

// options contains Uniq configuration.
type options struct {
	serializer serializer.Serializer
}

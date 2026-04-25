// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=defaultBuilderOptions --output=default_options_gen.go --option-type=DefaultBuilderOption --option-prefix=DefaultBuilder

// defaultBuilderOptions holds configuration for the Default builder.
type defaultBuilderOptions struct {
	errorConverter ErrorConverter
}

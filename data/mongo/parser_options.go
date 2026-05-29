// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=parserOptions --output=parser_options_gen.go --option-type=ParserOption

import (
	"reflect"
	"time"
)

// Default tag names recognized by [Parser].
const (
	// DefaultParserBSONTagName is the struct tag inspected for BSON field metadata.
	DefaultParserBSONTagName = "bson"

	// DefaultParserEncryptionTagName is the struct tag inspected for
	// per-field encryption metadata.
	DefaultParserEncryptionTagName = "encryption"
)

// parserOptions holds [Parser] configuration. All fields are populated
// by generated [ParserOption] setters in parser_options_gen.go.
type parserOptions struct {
	update            bool                              `opt:"ParserUpdate"`
	shouldEncrypt     bool                              `opt:"ParserEncryption"`
	encryptionModels  map[reflect.Type]*EncryptionModel `opt:"ParserEncryptionModels" optgen:"default=make(map[reflect.Type]*EncryptionModel)"`
	bsonTagName       string                            `opt:"ParserBSONTagName" optgen:"default=DefaultParserBSONTagName"`
	encryptionTagName string                            `opt:"ParserEncryptionTagName" optgen:"default=DefaultParserEncryptionTagName"`
	cacheSize         int                               `opt:"ParserCacheSize" optgen:"default=MaxCacheSize" optval:"positive"`
	cacheEvictionAge  time.Duration                     `opt:"ParserCacheEvictionAge" optval:"positive"`
}

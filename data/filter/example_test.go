// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/mongo"
)

// Example demonstrates the typical Parser → Translator chain for a CEL
// filter expression targeted at MongoDB.
func Example() {
	parser, err := filter.NewParser()
	if err != nil {
		log.Fatal(err)
	}

	ast, err := parser.Parse(context.Background(), `name == "John"`)
	if err != nil {
		log.Fatal(err)
	}

	trans, err := mongo.NewTranslator(
		filter.WithAllowedFields("name"),
	)
	if err != nil {
		log.Fatal(err)
	}

	bsonFilter, err := trans.Translate(ast)
	if err != nil {
		log.Fatal(err)
	}

	out, _ := json.Marshal(bsonFilter)
	fmt.Println(string(out))
	// Output: {"name":"John"}
}

// ExampleNewTranslatorContext_untrustedInput shows the construction-time
// guard that rejects [filter.WithUntrustedInput] unless paired with a
// non-empty [filter.WithAllowedFields] allow-list. The misconfiguration
// surfaces at boot rather than on the first request.
func ExampleNewTranslatorContext_untrustedInput() {
	_, err := mongo.NewTranslator(filter.WithUntrustedInput())
	fmt.Println(err)
	// Output: allowlist is required for untrusted input
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package orderby_test

import (
	"context"
	"fmt"
	"log"

	"github.com/altessa-s/go-atlas/data/orderby"
	"github.com/altessa-s/go-atlas/data/orderby/translators/mongo"
)

// Example demonstrates the typical Parser → Translator chain for an
// AIP-132 order_by string targeted at MongoDB.
func Example() {
	parser, err := orderby.NewParser()
	if err != nil {
		log.Fatal(err)
	}

	ob, err := parser.Parse(context.Background(), "create_time desc, slug")
	if err != nil {
		log.Fatal(err)
	}

	trans, err := mongo.NewTranslator(
		orderby.WithAllowedFields("create_time", "slug"),
	)
	if err != nil {
		log.Fatal(err)
	}

	sort, err := trans.Translate(ob)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(sort)
	// Output: {"create_time":{"$numberInt":"-1"},"slug":{"$numberInt":"1"}}
}

// ExampleNewTranslatorContext_untrustedInput shows the construction-time
// guard that rejects WithUntrustedInput unless paired with a non-empty
// allow-list. The misconfiguration surfaces at boot rather than on the
// first request.
func ExampleNewTranslatorContext_untrustedInput() {
	_, err := mongo.NewTranslator(orderby.WithUntrustedInput())
	fmt.Println(err)
	// Output: allowlist is required for untrusted input
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongo provides a translator that converts filter AST nodes to MongoDB bson.M filters.
//
// The translator implements the filter.Visitor interface and supports all standard CEL
// operations including comparisons, logical operators, membership tests, and string functions.
//
// # Basic Usage
//
//	parser, _ := filter.NewParser()
//	ast, _ := parser.Parse(ctx, `name == "John" && age >= 18`)
//
//	trans := mongo.NewTranslator()
//	bsonFilter, _ := trans.Translate(ast)
//	// Result: {"$and": [{"name": "John"}, {"age": {"$gte": 18}}]}
//
// # With Options
//
//	trans := mongo.NewTranslator(
//	    filter.WithAllowedFields("name", "age", "email"),
//	    filter.WithFieldMapping(map[string]string{
//	        "userName": "user_name",
//	    }),
//	    filter.WithMaxDepth(10),
//	)
//
// # Supported Operations
//
// Comparison: ==, !=, <, >, <=, >=
// Logical: &&, ||, !
// Membership: in, has()
// String: contains(), startsWith(), endsWith(), matches()
// Size: size()
package mongo

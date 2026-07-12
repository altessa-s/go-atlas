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
//	trans, err := mongo.NewTranslator()
//	if err != nil {
//	    return err
//	}
//	bsonFilter, _ := trans.Translate(ast)
//	// Result: {"$and": [{"name": "John"}, {"age": {"$gte": 18}}]}
//
// # With Options
//
//	trans, err := mongo.NewTranslator(
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
//
// # Security
//
// matches() passes the user-supplied pattern through to MongoDB's $regex.
// The pattern is validated host-side with Go's RE2 engine (compile check +
// length cap), but MongoDB executes it with its own PCRE-family engine,
// which — unlike RE2 — can backtrack catastrophically. A pathological
// pattern within the length cap can therefore still burn CPU on the
// database server (DB-side ReDoS). Expose matches() only to trusted
// callers, or tighten the cap with [filter.WithMaxRegexLength]. The
// contains(), startsWith(), and endsWith() operators escape their argument
// with regexp.QuoteMeta and are not affected.
package mongo

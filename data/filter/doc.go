// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package filter provides CEL expression parsing and translation to database-specific filters.
//
// The package parses Common Expression Language (CEL) expressions into an intermediate AST
// representation, which can then be translated to database-specific query formats using
// the visitor pattern.
//
// # Supported Operations
//
// Comparison operators:
//   - == (equal)
//   - != (not equal)
//   - < (less than)
//   - <= (less than or equal)
//   - > (greater than)
//   - >= (greater than or equal)
//
// Logical operators:
//   - && (and)
//   - || (or)
//   - ! (not)
//
// Membership operators:
//   - in (value in list)
//   - has() (field exists)
//
// String functions:
//   - contains() - substring match
//   - startsWith() - prefix match
//   - endsWith() - suffix match
//   - matches() - regex match
//
// Other:
//   - size() - array/string length
//   - timestamp() - parse RFC3339 timestamp
//
// # Basic Usage
//
//	// Create a parser
//	parser, err := filter.NewParser()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Parse a CEL expression
//	ast, err := parser.Parse(ctx, `name == "John" && age >= 18`)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Create a MongoDB translator
//	trans := mongo.NewTranslator()
//
//	// Translate to bson.M
//	bsonFilter, err := trans.Translate(ast)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// Result: {"$and": [{"name": "John"}, {"age": {"$gte": 18}}]}
//
// # Security Features
//
// Field Allowlist - restrict which fields can be queried:
//
//	trans := mongo.NewTranslator(
//	    filter.WithAllowedFields("name", "age", "email"),
//	)
//
// Depth Limit - protect against DoS via deeply nested expressions:
//
//	trans := mongo.NewTranslator(
//	    filter.WithMaxDepth(10),
//	)
//
// # Field Mapping
//
// Map CEL field names to database column names:
//
//	trans := mongo.NewTranslator(
//	    filter.WithFieldMapping(map[string]string{
//	        "userName":  "user_name",
//	        "createdAt": "created_at",
//	    }),
//	)
//
// # Translation Examples
//
// Simple comparisons:
//
//	name == "John"              → {"name": "John"}
//	age >= 18                   → {"age": {"$gte": 18}}
//	status != "deleted"         → {"status": {"$ne": "deleted"}}
//
// Logical operators:
//
//	name == "John" && age >= 18 → {"$and": [{"name": "John"}, {"age": {"$gte": 18}}]}
//	status == "a" || status == "b" → {"$or": [{"status": "a"}, {"status": "b"}]}
//	!active                     → {"active": {"$ne": true}}
//
// Membership:
//
//	status in ["active", "pending"] → {"status": {"$in": ["active", "pending"]}}
//	has(user.email)                 → {"user.email": {"$exists": true}}
//
// String functions:
//
//	name.contains("oh")      → {"name": {"$regex": "oh"}}
//	name.startsWith("J")     → {"name": {"$regex": "^J"}}
//	name.endsWith("n")       → {"name": {"$regex": "n$"}}
//	name.matches("^[A-Z].*") → {"name": {"$regex": "^[A-Z].*"}}
//
// Size comparison:
//
//	tags.size() == 3 → {"tags": {"$size": 3}}
//
// Nested fields:
//
//	address.city == "NYC" → {"address.city": "NYC"}
//
// Timestamps:
//
//	created_at >= timestamp("2024-01-01T00:00:00Z") → {"created_at": {"$gte": <time.Time>}}
//
// # Parser Caching
//
// The parser includes an LRU cache for parsed expressions:
//
//	// Custom cache size
//	parser, _ := filter.NewParser(filter.WithParserCacheSize(500))
//
//	// Disable caching
//	parser, _ := filter.NewParser(filter.WithParserNoCache())
//
// # Architecture
//
// The package uses a visitor pattern for translation:
//
//  1. Parser: Converts CEL expression string → AST nodes
//  2. AST Nodes: LiteralNode, IdentNode, BinaryOpNode, UnaryOpNode, CallNode, ListNode
//  3. Visitor: Interface implemented by translators
//  4. Translator: Converts AST → database-specific format (e.g., bson.M for MongoDB)
//
// This architecture allows adding new database backends by implementing the Visitor interface.
package filter

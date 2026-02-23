// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package lua provides a translator that converts filter AST nodes to Lua boolean expressions
// for use in Redis EVAL scripts. RedisJSON data accessed via cjson.decode() is represented
// as Lua tables, and the generated expressions filter these tables.
//
// # Basic Usage
//
//	parser, _ := filter.NewParser()
//	ast, _ := parser.Parse(`name == "John" && age >= 18`)
//
//	trans := lua.NewTranslator("d")
//	expr, _ := trans.Translate(ast)
//	// Result: `((d["name"] == "John") and (d["age"] >= 18))`
//
// # With Options
//
//	trans := lua.NewTranslator("item",
//	    filter.WithAllowedFields("name", "age", "status"),
//	    filter.WithFieldMapping(map[string]string{
//	        "userName": "user_name",
//	    }),
//	    filter.WithMaxDepth(10),
//	)
//
// # Supported Operations
//
// Comparison: ==, !=, <, >, <=, >= (maps to Lua ==, ~=, <, >, <=, >=)
// Logical: &&, ||, ! (maps to Lua and, or, not)
// Membership: in (expanded to OR chain)
// String: contains(), startsWith(), endsWith() (uses string.find, string.sub)
// Collection: size() (uses Lua # operator)
// Existence: has() (maps to ~= nil check)
//
// # Unsupported Operations
//
// matches() returns ErrUnsupportedOperation (Lua patterns are not PCRE/RE2 compatible).
//
// # Nested Fields
//
// Dotted field names are translated to chained bracket access:
//
//	address.city → d["address"]["city"]
package lua

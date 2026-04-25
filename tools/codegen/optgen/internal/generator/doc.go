// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package generator produces Go source files containing functional-option
// functions for struct types annotated with opt tags.
//
// Code generation is delegated to the plugin system: each struct field is
// matched to a [plugin.FieldPlugin] that emits the corresponding With*
// function body, type definitions, and helper code. The generator collects
// those fragments, resolves imports, and renders the final file through a
// text/template.
//
// Importing the builtin and fields plugin packages triggers init-time
// registration of all built-in field handlers, transforms, guards, checks,
// and post-processors.
//
// # Usage
//
//	gen := generator.New()
//	code, err := gen.Generate(generator.GenerateInput{
//	    PackageName: "mypkg",
//	    TypeName:    "Options",
//	    OptionType:  "Option",
//	    Fields:      fields,
//	    Imports:     imports,
//	})
package generator

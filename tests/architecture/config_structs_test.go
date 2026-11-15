// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// allowedPublicConfigStructs lists the exported `*Config` structs with
// exported fields that are deliberately kept outside config/ and proto/, keyed
// by "<package dir>.<type>". Each entry states why the type is not a set of
// caller-filled tunables, which the options rule would require to be generated
// functional options instead.
var allowedPublicConfigStructs = map[string]string{
	// SPI value carrier: the leadelect core assembles it in Leader.Start and
	// hands it, notification channels included, to Provider.Start. Callers
	// never fill it; see the type's godoc.
	"coord/leadelect/providers.Config": "SPI value carrier between the elector core and a provider",

	// Serialized document schema: the JSON validation-rules file an operator
	// writes, decoded as a whole (json tags on every field), like the YAML
	// schemas under config/.
	"auth/oidc.ServiceConfig":                 "JSON document schema",
	"auth/oidc.ValidationRulesConfig":         "JSON document schema",
	"auth/oidc.ScopesValidationConfig":        "JSON document schema",
	"auth/oidc.TokenLifetimeValidationConfig": "JSON document schema",
}

// TestNoPublicConfigStructs enforces the options rule of AGENTS.md: a package
// is configured through generated functional options, never through a public
// `Config` struct with exported fields. YAML-facing schemas live in config/
// and generated messages in proto/; both are out of scope.
func TestNoPublicConfigStructs(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	found := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			skipped := strings.HasPrefix(entry.Name(), ".") || entry.Name() == "vendor" || entry.Name() == "graphify-out" ||
				rel == "config" || rel == "proto"
			if path != root && skipped {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, name := range publicConfigStructs(file) {
			key := filepath.ToSlash(filepath.Dir(rel)) + "." + name
			found[key] = true
			if _, allowed := allowedPublicConfigStructs[key]; !allowed {
				t.Errorf("%s: exported struct %s has exported fields; configure the package through generated functional options "+
					"(AGENTS.md, Options pattern), or rename the type if it is a data record rather than configuration", rel, name)
			}
		}
		return nil
	})
	require.NoError(t, err)

	for _, key := range slices.Sorted(maps.Keys(allowedPublicConfigStructs)) {
		if !found[key] {
			t.Errorf("allowedPublicConfigStructs lists %s, which no longer exists or no longer has exported fields; remove the entry", key)
		}
	}
}

// publicConfigStructs returns the exported struct types of file whose name
// ends in "Config" and that declare at least one exported field. An embedded
// field counts by the name of its type.
func publicConfigStructs(file *ast.File) []string {
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typ, ok := spec.(*ast.TypeSpec)
			if !ok || !typ.Name.IsExported() || !strings.HasSuffix(typ.Name.Name, "Config") {
				continue
			}
			if st, ok := typ.Type.(*ast.StructType); ok && hasExportedField(st) {
				names = append(names, typ.Name.Name)
			}
		}
	}
	return names
}

func hasExportedField(st *ast.StructType) bool {
	for _, field := range st.Fields.List {
		for _, name := range field.Names {
			if name.IsExported() {
				return true
			}
		}
		if len(field.Names) == 0 && embeddedTypeName(field.Type).IsExported() {
			return true
		}
	}
	return false
}

// embeddedTypeName returns the identifier an embedded field is promoted under:
// the type name, without its package, pointer or type arguments.
func embeddedTypeName(expr ast.Expr) *ast.Ident {
	for {
		switch typ := expr.(type) {
		case *ast.Ident:
			return typ
		case *ast.SelectorExpr:
			return typ.Sel
		case *ast.StarExpr:
			expr = typ.X
		case *ast.ParenExpr:
			expr = typ.X
		case *ast.IndexExpr:
			expr = typ.X
		case *ast.IndexListExpr:
			expr = typ.X
		default:
			return ast.NewIdent("_")
		}
	}
}

func TestPublicConfigStructs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, source string
		want         []string
	}{
		{name: "exported fields", source: `type ServerConfig struct { Addr string }`, want: []string{"ServerConfig"}},
		{name: "bare Config", source: `type Config struct { Addr string }`, want: []string{"Config"}},
		{name: "grouped declaration", source: "type (\n A struct{ X int }\n RetryConfig struct { Max int }\n)", want: []string{"RetryConfig"}},
		{name: "generic struct", source: `type PoolConfig[T any] struct { Items []T }`, want: []string{"PoolConfig"}},
		{name: "mixed fields", source: `type LeaseConfig struct { key string; TTL int }`, want: []string{"LeaseConfig"}},
		{name: "embedded exported type", source: `type ClientConfig struct { Base }`, want: []string{"ClientConfig"}},
		{name: "embedded qualified pointer", source: `type ClientConfig struct { *pkg.Base[int] }`, want: []string{"ClientConfig"}},
		{name: "unexported fields only", source: `type SpanConfig struct { kind int }`},
		{name: "embedded unexported type", source: `type SpanConfig struct { spanOptions }`},
		{name: "empty struct", source: `type MarkerConfig struct{}`},
		{name: "unexported type", source: `type serverConfig struct { Addr string }`},
		{name: "other suffix", source: `type ConfigSource struct { Path string }`},
		{name: "not a struct", source: "type LoaderConfig interface { Load() }\ntype NameConfig string"},
		{name: "alias", source: `type ServerConfig = other.Settings`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture\n"+tt.source, parser.SkipObjectResolution)
			require.NoError(t, err)
			require.Equal(t, tt.want, publicConfigStructs(file))
		})
	}
}

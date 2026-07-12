// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package parser_test

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/parser"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"

	goparser "go/parser"
)

// stubCheck is a minimal CheckPlugin registered so that optcheck validation is
// enforced during tests (with an empty registry the parser skips validation).
type stubCheck struct {
	plugin.CheckBase
}

func (*stubCheck) Generate(_ plugin.GenerationContext, _ model.OptField, _, _, _ string) []string {
	return nil
}

func init() {
	plugin.Register(
		&stubCheck{CheckBase: plugin.NewCheckBase("stubcheck", 10, plugin.RequiresValue())},
		&stubCheck{CheckBase: plugin.NewCheckBase("stubflag", 20)},
	)
}

// src replaces "~" with backticks so fixtures can carry struct tags inside raw
// string literals.
func src(s string) string {
	return strings.ReplaceAll(s, "~", "`")
}

// parsePackage parses named Go sources into a single *ast.Package suitable for
// parser.FindOptFields.
//
//nolint:staticcheck // SA1019: the parser API intentionally operates on go/ast packages.
func parsePackage(t *testing.T, files map[string]string) *ast.Package {
	t.Helper()

	fset := token.NewFileSet()
	pkg := &ast.Package{Files: make(map[string]*ast.File, len(files))}
	for name, source := range files {
		file, err := goparser.ParseFile(fset, name, src(source), goparser.ParseComments)
		require.NoError(t, err)
		pkg.Name = file.Name.Name
		pkg.Files[name] = file
	}
	return pkg
}

// mustParseExpr parses a Go type expression.
func mustParseExpr(t *testing.T, expr string) ast.Expr {
	t.Helper()

	parsed, err := goparser.ParseExpr(expr)
	require.NoError(t, err)
	return parsed
}

// findTypeSpec locates a TypeSpec by name in the parsed package.
//
//nolint:staticcheck // SA1019: the parser API intentionally operates on go/ast packages.
func findTypeSpec(t *testing.T, pkg *ast.Package, name string) *ast.TypeSpec {
	t.Helper()

	var found *ast.TypeSpec
	for _, file := range pkg.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			if ts, ok := n.(*ast.TypeSpec); ok && ts.Name.Name == name {
				found = ts
				return false
			}
			return true
		})
	}
	require.NotNil(t, found, "type %s not found in fixture", name)
	return found
}

func fieldByOption(t *testing.T, fields []model.OptField, optionName string) model.OptField {
	t.Helper()

	for _, f := range fields {
		if f.OptionName == optionName {
			return f
		}
	}
	require.Failf(t, "field not found", "no field with option name %q", optionName)
	return model.OptField{}
}

const mainFixture = `
package sample

import (
	"time"

	vaultApi "github.com/hashicorp/vault/api"
)

type options struct {
	// name is the human-readable name.
	name    string           ~opt:"Name"~
	timeout time.Duration    ~optgen:"default=DefaultTimeout"~
	skipped string           ~opt:"-"~
	keep    int              ~opt:"-" optgen:"default=42"~
	client  *vaultApi.Client ~opt:"Client"~
	tags    []string         ~opt:"Tags"~
	plain   bool
}
`

func TestFindOptFields_TaggedFields(t *testing.T) {
	t.Parallel()

	pkg := parsePackage(t, map[string]string{"options.go": mainFixture})

	result, err := parser.FindOptFields(pkg, "options", false)
	require.NoError(t, err)

	names := make([]string, 0, len(result.Fields))
	for _, f := range result.Fields {
		names = append(names, f.OptionName)
	}
	// Sorted by OptionName; "skipped" (opt:"-") and untagged "plain" excluded.
	require.Equal(t, []string{"Client", "Keep", "Name", "Tags", "Timeout"}, names)

	name := fieldByOption(t, result.Fields, "Name")
	require.Equal(t, "name", name.FieldName)
	require.Equal(t, "string", name.Type)
	require.Equal(t, "name is the human-readable name.", name.Doc)
	require.False(t, name.IsNilable)
	require.False(t, name.DefaultOnly)

	timeout := fieldByOption(t, result.Fields, "Timeout")
	require.Equal(t, "time.Duration", timeout.Type)
	require.Equal(t, "DefaultTimeout", timeout.Default)

	keep := fieldByOption(t, result.Fields, "Keep")
	require.True(t, keep.DefaultOnly, "opt:\"-\" with default must be kept as DefaultOnly")
	require.Equal(t, "42", keep.Default)

	client := fieldByOption(t, result.Fields, "Client")
	require.Equal(t, "*vaultApi.Client", client.Type)
	require.True(t, client.IsNilable)
	require.False(t, client.IsSlice)

	tags := fieldByOption(t, result.Fields, "Tags")
	require.True(t, tags.IsSlice)
	require.Equal(t, "string", tags.ElemType)
	require.True(t, tags.IsNilable)

	require.Equal(t, map[string]model.ImportInfo{
		"time":     {Path: "time"},
		"vaultApi": {Path: "github.com/hashicorp/vault/api", Alias: "vaultApi"},
	}, result.Imports)
	require.False(t, result.GenericInfo.IsGeneric())
}

func TestFindOptFields_ProcessAllFields(t *testing.T) {
	t.Parallel()

	pkg := parsePackage(t, map[string]string{"options.go": mainFixture})

	result, err := parser.FindOptFields(pkg, "options", true)
	require.NoError(t, err)

	names := make([]string, 0, len(result.Fields))
	for _, f := range result.Fields {
		names = append(names, f.OptionName)
	}
	// "plain" is now included with a derived option name; "skipped" stays excluded.
	require.Equal(t, []string{"Client", "Keep", "Name", "Plain", "Tags", "Timeout"}, names)
}

func TestFindOptFields_StructNotFound(t *testing.T) {
	t.Parallel()

	pkg := parsePackage(t, map[string]string{"options.go": mainFixture})

	result, err := parser.FindOptFields(pkg, "missing", false)
	require.NoError(t, err)
	require.Empty(t, result.Fields)
	require.Empty(t, result.Imports)
}

func TestFindOptFields_NonStructType(t *testing.T) {
	t.Parallel()

	pkg := parsePackage(t, map[string]string{"options.go": `
package sample

type options int
`})

	result, err := parser.FindOptFields(pkg, "options", false)
	require.NoError(t, err)
	require.Empty(t, result.Fields)
}

func TestFindOptFields_GenericStruct(t *testing.T) {
	t.Parallel()

	pkg := parsePackage(t, map[string]string{"options.go": `
package sample

type options[T any, U comparable] struct {
	value T        ~opt:"Value"~
	pair  map[T]U  ~opt:"Pair"~
}
`})

	result, err := parser.FindOptFields(pkg, "options", false)
	require.NoError(t, err)

	require.Equal(t, []model.TypeParam{
		{Name: "T", Constraint: "any"},
		{Name: "U", Constraint: "comparable"},
	}, result.GenericInfo.TypeParams)

	require.Equal(t, "T", fieldByOption(t, result.Fields, "Value").Type)
	require.Equal(t, "map[T]U", fieldByOption(t, result.Fields, "Pair").Type)
}

func TestFindOptFields_Modifiers(t *testing.T) {
	t.Parallel()

	pkg := parsePackage(t, map[string]string{"options.go": `
package sample

type options struct {
	name string ~opt:"Name" optval:"lower,trimspaces"~
}
`})

	result, err := parser.FindOptFields(pkg, "options", false)
	require.NoError(t, err)
	require.Len(t, result.Fields, 1)

	field := result.Fields[0]
	require.Equal(t, []string{"lower", "trimspaces"}, field.Modifiers)
	require.True(t, field.HasModifiers)
	require.True(t, field.HasModifier("lower"))
	require.False(t, field.HasModifier("upper"))
}

func TestFindOptFields_Metadata(t *testing.T) {
	t.Parallel()

	pkg := parsePackage(t, map[string]string{"options.go": `
package sample

type options struct {
	count int ~optgen:"default=10,append,err=custom"~
}
`})

	result, err := parser.FindOptFields(pkg, "options", false)
	require.NoError(t, err)
	require.Len(t, result.Fields, 1)

	field := result.Fields[0]
	require.Equal(t, "10", field.Default)
	require.Equal(t, map[string]string{"append": "true", "err": "custom"}, field.Metadata)
}

func TestFindOptFields_LineCommentDoc(t *testing.T) {
	t.Parallel()

	pkg := parsePackage(t, map[string]string{"options.go": `
package sample

import _ "embed"

type options struct {
	data string ~opt:"Data"~ // raw payload
}
`})

	result, err := parser.FindOptFields(pkg, "options", false)
	require.NoError(t, err)
	require.Len(t, result.Fields, 1)
	require.Equal(t, "raw payload", result.Fields[0].Doc)

	// Blank imports are converted to regular imports for generated code.
	require.Equal(t, map[string]model.ImportInfo{
		"embed": {Path: "embed", Alias: ""},
	}, result.Imports)
}

func TestFindOptFields_ImportsOnlyFromDefiningFile(t *testing.T) {
	t.Parallel()

	pkg := parsePackage(t, map[string]string{
		"a.go": `
package sample

import "time"

type options struct {
	timeout time.Duration ~opt:"Timeout"~
}
`,
		"b.go": `
package sample

import "os"

type other struct {
	f *os.File
}
`,
	})

	result, err := parser.FindOptFields(pkg, "options", false)
	require.NoError(t, err)
	require.Equal(t, map[string]model.ImportInfo{
		"time": {Path: "time"},
	}, result.Imports)
}

func TestFindOptFields_OptCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		source  string
		wantErr bool
		checks  map[string]string
	}{
		{
			name: "valid keys",
			source: `
package sample

type options struct {
	field string ~opt:"Field" optcheck:"stubcheck=5,stubflag"~
}
`,
			checks: map[string]string{"stubcheck": "5", "stubflag": "true"},
		},
		{
			name: "unknown key",
			source: `
package sample

type options struct {
	field string ~opt:"Field" optcheck:"nosuchcheck"~
}
`,
			wantErr: true,
		},
		{
			name: "explicit empty value fails required-value check",
			source: `
package sample

type options struct {
	field string ~opt:"Field" optcheck:"stubcheck="~
}
`,
			wantErr: true,
		},
		{
			name: "bare key fails required-value check",
			source: `
package sample

type options struct {
	field string ~opt:"Field" optcheck:"stubcheck"~
}
`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pkg := parsePackage(t, map[string]string{"options.go": tc.source})
			result, err := parser.FindOptFields(pkg, "options", false)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, result.Fields, 1)
			require.Equal(t, tc.checks, result.Fields[0].Checks)
		})
	}
}

func TestFindNormalizationMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "exported pointer receiver",
			source: `
package sample

type options struct{}

func (o *options) Normalization() {}
`,
			want: "Normalization",
		},
		{
			name: "unexported value receiver",
			source: `
package sample

type options struct{}

func (o options) normalization() {}
`,
			want: "normalization",
		},
		{
			name: "method on another type",
			source: `
package sample

type options struct{}
type other struct{}

func (o *other) Normalization() {}
`,
			want: "",
		},
		{
			name: "plain function is ignored",
			source: `
package sample

type options struct{}

func Normalization() {}
`,
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pkg := parsePackage(t, map[string]string{"options.go": tc.source})
			require.Equal(t, tc.want, parser.FindNormalizationMethod(pkg, "options"))
		})
	}
}

func TestTypeToString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expr string
		want string
	}{
		{expr: "int", want: "int"},
		{expr: "pkg.Type", want: "pkg.Type"},
		{expr: "*pkg.Type", want: "*pkg.Type"},
		{expr: "[]string", want: "[]string"},
		{expr: "map[string]int", want: "map[string]int"},
		{expr: "interface{}", want: "any"},
		{expr: "any", want: "any"},
		{expr: "func(int) error", want: "func(...)"},
		{expr: "chan int", want: "chan int"},
		{expr: "chan<- int", want: "chan<- int"},
		{expr: "<-chan int", want: "<-chan int"},
		{expr: "struct{}", want: "struct{}"},
		{expr: "(int)", want: "int"},
		{expr: "Container[T]", want: "Container[T]"},
		{expr: "Map[K, V]", want: "Map[K, V]"},
	}

	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, parser.TypeToString(mustParseExpr(t, tc.expr)))
		})
	}
}

func TestTypeToString_ConstructedNodes(t *testing.T) {
	t.Parallel()

	require.Equal(t, "...int", parser.TypeToString(&ast.Ellipsis{Elt: ast.NewIdent("int")}))
	// Unknown node kinds degrade to "any".
	require.Equal(t, "any", parser.TypeToString(&ast.BasicLit{Value: "42"}))
}

func TestIsNilableType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expr string
		want bool
	}{
		{expr: "*int", want: true},
		{expr: "[]int", want: true},
		{expr: "map[string]int", want: true},
		{expr: "chan int", want: true},
		{expr: "func()", want: true},
		{expr: "interface{}", want: true},
		{expr: "int", want: false},
		{expr: "pkg.Type", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, parser.IsNilableType(mustParseExpr(t, tc.expr)))
		})
	}
}

func TestIsInterfaceType(t *testing.T) {
	t.Parallel()

	require.True(t, parser.IsInterfaceType(mustParseExpr(t, "interface{}")))
	// The "any" identifier is not recognized as an interface literal.
	require.False(t, parser.IsInterfaceType(mustParseExpr(t, "any")))
	require.False(t, parser.IsInterfaceType(mustParseExpr(t, "int")))
}

func TestIsStdLibType(t *testing.T) {
	t.Parallel()

	require.True(t, parser.IsStdLibType("context.Context"))
	require.True(t, parser.IsStdLibType("*context.Context"))
	require.True(t, parser.IsStdLibType("[]context.Context"))
	require.False(t, parser.IsStdLibType("time.Duration"))
	require.False(t, parser.IsStdLibType(""))
}

func TestExtractTypeParams(t *testing.T) {
	t.Parallel()

	pkg := parsePackage(t, map[string]string{"types.go": `
package sample

import "io"

type plain struct{}
type single[T any] struct{}
type shared[T, U comparable] struct{}
type constrained[S io.Reader] struct{}
`})

	require.Nil(t, parser.ExtractTypeParams(nil))
	require.Nil(t, parser.ExtractTypeParams(findTypeSpec(t, pkg, "plain")))

	require.Equal(t, []model.TypeParam{{Name: "T", Constraint: "any"}},
		parser.ExtractTypeParams(findTypeSpec(t, pkg, "single")))

	require.Equal(t, []model.TypeParam{
		{Name: "T", Constraint: "comparable"},
		{Name: "U", Constraint: "comparable"},
	}, parser.ExtractTypeParams(findTypeSpec(t, pkg, "shared")))

	require.Equal(t, []model.TypeParam{{Name: "S", Constraint: "io.Reader"}},
		parser.ExtractTypeParams(findTypeSpec(t, pkg, "constrained")))
}

func TestExtractImportsFromType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		expr    string
		imports map[string]model.ImportInfo
		want    []string
	}{
		{
			name:    "plain stdlib package",
			expr:    "time.Duration",
			imports: map[string]model.ImportInfo{"time": {Path: "time"}},
			want:    []string{"time"},
		},
		{
			name: "versioned import keeps identifier alias",
			expr: "*redis.Client",
			imports: map[string]model.ImportInfo{
				"redis": {Path: "github.com/redis/go-redis/v9"},
			},
			// derivePackageName yields "go-redis", so the identifier is preserved
			// as an explicit alias in the "alias:path" form.
			want: []string{"redis:github.com/redis/go-redis/v9"},
		},
		{
			name: "explicit alias preserved",
			expr: "vaultApi.Client",
			imports: map[string]model.ImportInfo{
				"vaultApi": {Path: "github.com/hashicorp/vault/api", Alias: "vaultApi"},
			},
			want: []string{"vaultApi:github.com/hashicorp/vault/api"},
		},
		{
			name: "fallback lookup by path segment",
			expr: "nats.Conn",
			imports: map[string]model.ImportInfo{
				"natsgo": {Path: "github.com/nats-io/nats.go", Alias: "natsgo"},
			},
			want: []string{"github.com/nats-io/nats.go"},
		},
		{
			name:    "context is resolved without imports map",
			expr:    "context.Context",
			imports: map[string]model.ImportInfo{},
			want:    []string{"context"},
		},
		{
			name:    "unknown package yields nothing",
			expr:    "foo.Bar",
			imports: map[string]model.ImportInfo{},
			want:    nil,
		},
		{
			name:    "duplicates are collapsed",
			expr:    "map[time.Time]time.Duration",
			imports: map[string]model.ImportInfo{"time": {Path: "time"}},
			want:    []string{"time"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := parser.ExtractImportsFromType(mustParseExpr(t, tc.expr), tc.imports)
			require.Equal(t, tc.want, got)
		})
	}
}

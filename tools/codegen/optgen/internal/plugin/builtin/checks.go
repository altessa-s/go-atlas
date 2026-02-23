// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package builtin

import (
	"cmp"
	"slices"
	"strings"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// CheckKind classifies a field into one of the categories that validation
// check plugins use to decide which code pattern to emit.
type CheckKind int

const (
	CheckKindOther CheckKind = iota
	CheckKindString
	CheckKindSlice
	CheckKindMap
	CheckKindPointer
)

type checkBuilder struct {
	ctx      plugin.GenerationContext
	field    model.OptField
	valueVar string
	lines    []string
}

func (b *checkBuilder) add(lines ...string) {
	b.lines = append(b.lines, lines...)
}

func indentLines(s, indent string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

// BuildChecks generates validation code for a field value.
//
// Supported checks (optcheck tag):
//   - required
//   - minlen=Expr (string/slice/map)
//   - maxlen=Expr (string/slice/map)
//   - oneof=[a,b,c] (string only; values are Go expressions, e.g. "a")
//
// The returned code is already indented for direct inclusion inside an option function body.
func BuildChecks(ctx plugin.GenerationContext, field model.OptField, kind CheckKind, valueVar string) string {
	if len(field.Checks) == 0 {
		return ""
	}

	b := checkBuilder{
		ctx:      ctx,
		field:    field,
		valueVar: valueVar,
	}

	type inv struct {
		key      string
		rawValue string
		spec     plugin.CheckSpec
		handler  plugin.CheckPlugin
	}
	invs := make([]inv, 0, len(field.Checks))
	for k, v := range field.Checks {
		h, spec, ok := plugin.FindCheckPlugin(k)
		if !ok {
			// Parser validation should have caught this. If not, ignore for backward compatibility.
			continue
		}
		invs = append(invs, inv{key: k, rawValue: v, spec: spec, handler: h})
	}
	if len(invs) == 0 {
		return ""
	}
	slices.SortFunc(invs, func(a, b inv) int {
		if c := cmp.Compare(a.spec.Priority, b.spec.Priority); c != 0 {
			return c
		}
		return cmp.Compare(a.key, b.key)
	})

	switch kind {
	case CheckKindPointer:
		for _, x := range invs {
			b.add(x.handler.Generate(ctx, field, "pointer", valueVar, x.rawValue)...)
		}
	case CheckKindString:
		for _, x := range invs {
			b.add(x.handler.Generate(ctx, field, "string", valueVar, x.rawValue)...)
		}
	case CheckKindSlice, CheckKindMap:
		kindName := "slice"
		if kind == CheckKindMap {
			kindName = "map"
		}
		for _, x := range invs {
			b.add(x.handler.Generate(ctx, field, kindName, valueVar, x.rawValue)...)
		}
	default:
		for _, x := range invs {
			b.add(x.handler.Generate(ctx, field, "other", valueVar, x.rawValue)...)
		}
	}

	if len(b.lines) == 0 {
		return ""
	}
	return indentLines(strings.Join(b.lines, "\n"), "\t\t") + "\n"
}

// KindForFieldType returns the CheckKind for a given field based on its type.
func KindForFieldType(field model.OptField) CheckKind {
	switch {
	case strings.HasPrefix(field.Type, "*"):
		return CheckKindPointer
	case field.Type == "string":
		return CheckKindString
	case field.IsSlice:
		return CheckKindSlice
	case strings.HasPrefix(field.Type, "map["):
		return CheckKindMap
	default:
		return CheckKindOther
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package builtin

import (
	"strings"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/model"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// OptionBaseData carries the minimal context needed by every option function
// template: the field being generated, the target struct and option type names,
// and generic type parameter declarations.
type OptionBaseData struct {
	model.OptField
	TypeName           string
	OptionType         string
	OptionReturnsError bool
	TypeParamsDecl     string // "[T any]" or "" for non-generic
	TypeParamsNames    string // "[T]" or "" for non-generic
}

// NewOptionBaseData creates base data for option template rendering.
func NewOptionBaseData(ctx plugin.GenerationContext, field model.OptField) OptionBaseData {
	return OptionBaseData{
		OptField:           field,
		TypeName:           ctx.TypeName,
		OptionType:         ctx.OptionType,
		OptionReturnsError: ctx.OptionReturnsError,
		TypeParamsDecl:     ctx.TypeParamsDecl(),
		TypeParamsNames:    ctx.TypeParamsNames(),
	}
}

// OptionData extends [OptionBaseData] with pre-rendered code fragments for each
// pipeline phase (guards, transforms, post-processors, checks) and the final
// value expression to assign to the struct field.
type OptionData struct {
	OptionBaseData
	GuardsCode      string
	TransformCode   string // Loop code for slice transforms (empty for scalars)
	PostProcessCode string
	ChecksCode      string
	ValueExpr       string // Expression for the value to assign (possibly transformed)
}

// buildPhaseCode generates code for modifiers in a specific phase.
func buildPhaseCode(ctx plugin.GenerationContext, field model.OptField, modifiers []plugin.ModifierPlugin, phase plugin.Phase, inputVar string) string {
	lines := make([]string, 0, len(modifiers))
	for _, mod := range modifiers {
		if mod.Phase() != phase {
			continue
		}
		result := mod.Generate(ctx, field, inputVar)
		if len(result.Code) > 0 {
			lines = append(lines, result.Code...)
		}
		if result.Final {
			break
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return indentLines(strings.Join(lines, "\n"), "\t\t") + "\n"
}

// BuildGuards generates guard code using ModifierPlugins with PhaseGuard.
func BuildGuards(ctx plugin.GenerationContext, field model.OptField, inputVar string) string {
	modifiers := plugin.CollectModifiers(field)
	return buildPhaseCode(ctx, field, modifiers, plugin.PhaseGuard, inputVar)
}

// BuildPostProcess generates post-processing code using ModifierPlugins with PhasePostProcess.
func BuildPostProcess(ctx plugin.GenerationContext, field model.OptField, valueVar string) string {
	modifiers := plugin.CollectModifiers(field)
	return buildPhaseCode(ctx, field, modifiers, plugin.PhasePostProcess, valueVar)
}

// transformResult holds the result of BuildTransform.
type transformResult struct {
	Code     string // Loop code for slice transforms (empty for scalars)
	ValueVar string // Variable name to use for assignment
}

// BuildTransform generates transform code for a field.
// For scalars with transforms that need a temp variable: returns Code with temp variable, ValueVar is "vv".
// For scalars with transforms that can be inlined: returns empty Code, ValueVar is the inline expression.
// For slices with string transforms: returns loop Code, ValueVar is "transformed".
// For fields without transforms: returns empty Code, ValueVar is inputVar.
// The needsTempVar parameter indicates whether downstream code (checks, postprocess) needs
// the transformed value in a variable.
func BuildTransform(ctx plugin.GenerationContext, field model.OptField, inputVar string, needsTempVar bool) transformResult {
	if !field.HasModifiers || !HasStringModifier(field.Modifiers) {
		return transformResult{ValueVar: inputVar}
	}

	if field.IsSlice && field.ElemType == "string" {
		// Generate transform loop for []string
		transform := BuildStringTransformChain(ctx, field, "val", field.Modifiers)
		var b strings.Builder
		b.Grow(100) // estimate
		b.WriteString("transformed := make([]string, len(")
		b.WriteString(inputVar)
		b.WriteString("))\nfor i, val := range ")
		b.WriteString(inputVar)
		b.WriteString(" {\n\ttransformed[i] = ")
		b.WriteString(transform)
		b.WriteString("\n}")
		return transformResult{
			Code:     indentLines(b.String(), "\t\t") + "\n",
			ValueVar: "transformed",
		}
	}

	if field.Type == "string" {
		transform := BuildStringTransformChain(ctx, field, inputVar, field.Modifiers)
		if needsTempVar {
			return transformResult{
				Code:     indentLines("vv := "+transform, "\t\t") + "\n",
				ValueVar: "vv",
			}
		}
		return transformResult{ValueVar: transform}
	}

	return transformResult{ValueVar: inputVar}
}

// hasChecksOrPostProcess checks if the field has any check or post-process modifiers
// that would require a temp variable to hold the transformed value.
func hasChecksOrPostProcess(field model.OptField) bool {
	modifiers := plugin.CollectModifiers(field)
	for _, mod := range modifiers {
		phase := mod.Phase()
		if phase == plugin.PhaseCheck || phase == plugin.PhasePostProcess {
			return true
		}
	}
	return false
}

// NewOptionData creates full option data with generated code for all phases.
func NewOptionData(ctx plugin.GenerationContext, field model.OptField, kind CheckKind, inputVar string) OptionData {
	// Determine if we need a temp variable for transformed value:
	// Required when checks or post-process modifiers need to reference it.
	needsTempVar := hasChecksOrPostProcess(field)
	tr := BuildTransform(ctx, field, inputVar, needsTempVar)
	return OptionData{
		OptionBaseData:  NewOptionBaseData(ctx, field),
		GuardsCode:      BuildGuards(ctx, field, inputVar),
		TransformCode:   tr.Code,
		PostProcessCode: BuildPostProcess(ctx, field, tr.ValueVar),
		ChecksCode:      BuildChecks(ctx, field, kind, tr.ValueVar),
		ValueExpr:       tr.ValueVar,
	}
}

// NewOptionDataWithGuardVar creates option data using guardVar for guards and inputVar for checks/postprocess.
// Use this when the guard must reference the function parameter (e.g., "v") but checks reference a derived variable.
func NewOptionDataWithGuardVar(ctx plugin.GenerationContext, field model.OptField, kind CheckKind, inputVar, guardVar string) OptionData {
	needsTempVar := hasChecksOrPostProcess(field)
	tr := BuildTransform(ctx, field, inputVar, needsTempVar)
	return OptionData{
		OptionBaseData:  NewOptionBaseData(ctx, field),
		GuardsCode:      BuildGuards(ctx, field, guardVar),
		TransformCode:   tr.Code,
		PostProcessCode: BuildPostProcess(ctx, field, tr.ValueVar),
		ChecksCode:      BuildChecks(ctx, field, kind, tr.ValueVar),
		ValueExpr:       tr.ValueVar,
	}
}

// NewOptionDataForFieldType creates option data with check kind inferred from field type.
func NewOptionDataForFieldType(ctx plugin.GenerationContext, field model.OptField, valueVar string) OptionData {
	return NewOptionData(ctx, field, KindForFieldType(field), valueVar)
}

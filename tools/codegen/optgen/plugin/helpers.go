// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugin

// BuildEarlyReturn generates an if-block that returns early from the option function
// when condition is true. If ctx.OptionReturnsError is set the block returns nil
// (no error); otherwise it uses a bare return. The returned slice contains the opening
// "if", the return statement, and the closing brace -- without leading indentation.
func BuildEarlyReturn(ctx GenerationContext, condition string) []string {
	if ctx.OptionReturnsError {
		return []string{
			"if " + condition + " {",
			"  return nil",
			"}",
		}
	}
	return []string{
		"if " + condition + " {",
		"  return",
		"}",
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build tools

package tools

import (
	_ "github.com/daixiang0/gci"
	_ "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
	_ "github.com/mibk/dupl"
	_ "github.com/securego/gosec/v2/cmd/gosec"
	_ "golang.org/x/vuln/cmd/govulncheck"
)

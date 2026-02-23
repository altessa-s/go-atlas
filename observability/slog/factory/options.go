// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	_ "github.com/altessa-s/go-atlas/core/runtime/appinfo"
)

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

// ModuleKey is the default attribute key for module prefixes.
const ModuleKey = "module"

// options contains Factory configuration.
type options struct {
	// prefixKey is the attribute key for prefix values.
	prefixKey string `optgen:"default=ModuleKey"`

	// prefixColors defines the colors for prefix attributes.
	prefixColors map[string][]int `optgen:"default=map[string][]int{ModuleKey: {46}}"`

	// enableMasking enables sensitive field masking.
	enableMasking bool

	// appName is the application name to include in logs.
	appName string `optgen:"default=appinfo.Name"`

	// appVersion is the application version to include in logs.
	appVersion string `optgen:"default=appinfo.Version"`

	// serviceId is the service ID to include in logs.
	serviceId string

	// levelVar is the dynamic log level variable.
	levelVar *slog.LevelVar
}

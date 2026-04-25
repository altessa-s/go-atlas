// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package convert

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ConfigToConfigConverter converts between structured configuration formats
// (YAML and TOML) without flattening to environment variables. The data model
// is preserved as-is; only the serialization format changes.
type ConfigToConfigConverter struct{}

// NewConfigToConfigConverter creates a new config to config converter.
func NewConfigToConfigConverter() *ConfigToConfigConverter {
	return &ConfigToConfigConverter{}
}

// Convert reads fromPath in fromFormat, then writes it to toPath in toFormat.
// Supported format values are "yaml" and "toml".
// #nosec G304 -- CLI tool, path from command-line arguments
func (conv *ConfigToConfigConverter) Convert(fromPath, toPath, fromFormat, toFormat string) error {
	// Read source file
	data, err := os.ReadFile(fromPath)
	if err != nil {
		return coreerrs.Wrapf(err, "failed to read config file %s", fromPath)
	}

	// Parse source format
	var configData map[string]any

	switch fromFormat {
	case formatYAML:
		if parseErr := yaml.Unmarshal(data, &configData); parseErr != nil {
			return coreerrs.WrapOperation(parseErr, "parse YAML")
		}
	case formatTOML:
		if parseErr := toml.Unmarshal(data, &configData); parseErr != nil {
			return coreerrs.WrapOperation(parseErr, "parse TOML")
		}
	default:
		return fmt.Errorf("unsupported source format: %s", fromFormat)
	}

	return conv.ConvertData(configData, toPath, toFormat)
}

// ConvertData writes pre-loaded configData to toPath in toFormat.
// This is used by the directory-merge path where files have already been
// parsed and merged.
func (conv *ConfigToConfigConverter) ConvertData(configData map[string]any, toPath, toFormat string) error {
	return writeConfigFile(toPath, toFormat, configData)
}

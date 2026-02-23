// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package convert

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// marshalConfig serializes data to the given format ("yaml" or "toml").
// Returns an error for unsupported formats.
func marshalConfig(format string, data any) ([]byte, error) {
	switch format {
	case formatYAML:
		out, err := yaml.Marshal(data)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "marshal YAML")
		}
		return out, nil
	case formatTOML:
		var builder strings.Builder
		encoder := toml.NewEncoder(&builder)
		if err := encoder.Encode(data); err != nil {
			return nil, coreerrs.WrapOperation(err, "marshal TOML")
		}
		return []byte(builder.String()), nil
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// writeConfigFile marshals data in the specified format and writes it to
// toPath with filePermission (0644).
func writeConfigFile(toPath string, format string, data any) error {
	out, err := marshalConfig(format, data)
	if err != nil {
		return err
	}
	if err := os.WriteFile(toPath, out, filePermission); err != nil {
		return coreerrs.Wrapf(err, "failed to write config file %s", toPath)
	}
	return nil
}

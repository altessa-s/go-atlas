// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nodeconfig

import (
	"regexp"
	"strings"

	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

var nodeIdRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Config represents the configuration for a service node.
// It contains identification information for the service instance.
//
// Example:
//
//	node := &nodeconfig.Config{Id: new("node-1")}
//	node.Normalize() // converts to uppercase
type Config struct {
	// Unique identifier for the service node
	// Must contain only alphanumeric characters, underscores, and hyphens
	Id *string `yaml:"id"`
}

// Normalize processes the node configuration to ensure consistent formatting.
// Converts the node ID to uppercase if present.
func (s *Config) Normalize() {
	if s.Id != nil {
		s.Id = new(strings.ToUpper(*s.Id))
	}
}

// Validate checks that the node configuration is valid.
// Returns an error if validation fails, nil otherwise.
func (s *Config) Validate() error {
	return validationconfig.ValidateStruct(s,
		validation.Field(&s.Id, validation.When(s.Id != nil, validation.Match(nodeIdRegex))),
	)
}

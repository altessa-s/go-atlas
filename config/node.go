// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"regexp"
	"strings"

	"github.com/altessa-s/go-atlas/core/types/ptr"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

var nodeIdRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Node represents the configuration for a service node.
// It contains identification information for the service instance.
//
// Example:
//
//	node := &config.Node{Id: ptr.Wrap("node-1")}
//	node.Normalize() // converts to uppercase
type Node struct {
	// Unique identifier for the service node
	// Must contain only alphanumeric characters, underscores, and hyphens
	Id *string `yaml:"id"`
}

// Normalize processes the node configuration to ensure consistent formatting.
// Converts the node ID to uppercase if present.
func (s *Node) Normalize() {
	if s.Id != nil {
		s.Id = ptr.Wrap(strings.ToUpper(*s.Id))
	}
}

// Validate checks that the node configuration is valid.
// Returns an error if validation fails, nil otherwise.
func (s *Node) Validate() error {
	return ValidateStruct(s,
		validation.Field(&s.Id, validation.When(s.Id != nil, validation.Match(nodeIdRegex))),
	)
}

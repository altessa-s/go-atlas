// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// brokerTemplatePath is the annotated YAML operators copy from when setting up
// a broker. It is documentation, not compiled input, so nothing links it to the
// struct it describes.
const brokerTemplatePath = "templates/broker.yaml"

// A field added to Outbox without a matching template entry is invisible to the
// operator reading the template: the knob exists, the YAML never mentions it,
// and the default silently rules. Nothing else in the build catches that —
// the template is commented-out prose that no test parses and no code embeds.
//
// Scoped to Outbox on purpose. The same drift exists for other config structs,
// and a repo-wide version of this check would fail on debt this change did not
// create.
func TestBrokerTemplate_DocumentsEveryOutboxField(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(brokerTemplatePath)
	require.NoError(t, err)
	template := string(raw)

	var missing []string
	for _, name := range yamlFieldNames(reflect.TypeOf(Outbox{})) {
		// The template is fully commented out, so match on the key as it would
		// appear once uncommented rather than on YAML structure.
		if !strings.Contains(template, name+":") {
			missing = append(missing, name)
		}
	}

	require.Empty(t, missing,
		"config.Outbox fields missing from %s: %v — add them so the template stays a complete reference",
		brokerTemplatePath, missing)
}

// The mirror image: a key in the template that no longer exists on the struct
// tells an operator to set something that is silently ignored.
func TestBrokerTemplate_HasNoStaleOutboxKeys(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(brokerTemplatePath)
	require.NoError(t, err)

	known := make(map[string]struct{})
	for _, name := range yamlFieldNames(reflect.TypeOf(Outbox{})) {
		known[name] = struct{}{}
	}

	// Only the outbox block is in scope; the file documents the whole broker.
	_, outboxBlock, found := strings.Cut(string(raw), "#  outbox:")
	require.True(t, found, "the outbox block must exist in %s", brokerTemplatePath)

	for _, line := range strings.Split(outboxBlock, "\n") {
		key, ok := templateKey(line)
		if !ok {
			continue
		}
		_, isKnown := known[key]
		require.True(t, isKnown,
			"%s documents %q, which is not a field of config.Outbox", brokerTemplatePath, key)
	}
}

// yamlFieldNames returns the yaml tag names of a struct's exported fields.
func yamlFieldNames(t reflect.Type) []string {
	names := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		tag := t.Field(i).Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		names = append(names, strings.Split(tag, ",")[0])
	}
	return names
}

// templateKey extracts the setting name from a commented template line such as
// `#    retryMaxAttempts: 10`, reporting false for prose and blank lines.
func templateKey(line string) (string, bool) {
	trimmed := strings.TrimLeft(line, "# \t")
	if trimmed == "" {
		return "", false
	}

	key, _, found := strings.Cut(trimmed, ":")
	if !found {
		return "", false
	}

	// Prose in a comment can contain a colon ("Example: ...", "Default value: ...").
	// A real key is a bare lowerCamelCase identifier, which rules those out.
	if key == "" || key[0] < 'a' || key[0] > 'z' {
		return "", false
	}
	for _, r := range key {
		isIdent := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
		if !isIdent {
			return "", false
		}
	}

	return key, true
}

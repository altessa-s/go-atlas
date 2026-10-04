// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3

import (
	"errors"
	"io"

	"gopkg.in/yaml.v3"

	"github.com/altessa-s/go-atlas/config/loader/backend"
)

// mergeTag is the YAML tag of a "<<" merge key.
const mergeTag = "!!merge"

// DecodeKeys reports the keys the YAML document sets, keeping each mapping
// key's source spelling ("0x10" stays "0x10") the way decoding into a
// string-keyed map does. Aliases are resolved and "<<" merge keys contribute
// their keys at lower precedence than the mapping's own keys.
func (b *Backend) DecodeKeys(reader io.Reader) (map[string]any, error) {
	var doc yaml.Node
	if err := yaml.NewDecoder(reader).Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	keys, _ := nodeKeys(&doc).(map[string]any)
	if keys == nil {
		keys = map[string]any{}
	}
	return keys, nil
}

// nodeKeys converts a YAML node into the presence tree.
func nodeKeys(n *yaml.Node) any {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil
		}
		return nodeKeys(n.Content[0])
	case yaml.AliasNode:
		return nodeKeys(n.Alias)
	case yaml.MappingNode:
		out := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			if k := n.Content[i]; k.Tag == mergeTag {
				mergeInto(out, n.Content[i+1])
			}
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			if k := n.Content[i]; k.Tag != mergeTag {
				out[k.Value] = nodeKeys(n.Content[i+1])
			}
		}
		return out
	case yaml.SequenceNode:
		out := make([]any, len(n.Content))
		for i, c := range n.Content {
			out[i] = nodeKeys(c)
		}
		return out
	default:
		if n.Tag == "!!null" {
			return nil
		}
		return n.Value
	}
}

// mergeInto adds the keys of a merge source — a mapping, an alias to one, or a
// sequence of them, where earlier sources win — without overriding keys
// already present.
func mergeInto(out map[string]any, src *yaml.Node) {
	if src.Kind == yaml.SequenceNode {
		for _, s := range src.Content {
			mergeInto(out, s)
		}
		return
	}
	m, ok := nodeKeys(src).(map[string]any)
	if !ok {
		return
	}
	for k, v := range m {
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
}

// Ensure Backend implements backend.KeyDecoder.
var _ backend.KeyDecoder = (*Backend)(nil)

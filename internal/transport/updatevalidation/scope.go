// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package updatevalidation

import (
	"maps"

	"github.com/altessa-s/go-aip/fieldmask"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// Tree builds the scope of the resolved mask. A nil tree covers the whole value.
func (s *Selection) Tree() fieldmask.FieldMask {
	paths := s.Mask.Paths()
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		names = append(names, path.Path)
	}
	return fieldmask.FromPaths(names...)
}

// MapKeyScope returns the selection of a concrete map entry.
func MapKeyScope(tree fieldmask.FieldMask, key protoreflect.MapKey) (fieldmask.FieldMask, bool) {
	if tree == nil {
		return nil, true
	}
	name := key.String()
	if name == "*" {
		for literal := range fieldmask.FromPaths("`*`") {
			name = literal
		}
	}
	exact, exactOK := tree[name]
	wildcard, wildcardOK := tree["*"]
	switch {
	case !exactOK:
		return wildcard, wildcardOK
	case !wildcardOK:
		return exact, true
	case exact == nil || wildcard == nil:
		return nil, true
	default:
		return unionScope(exact, wildcard), true
	}
}

// unionScope combines two descendant selections without changing either input.
func unionScope(a, b fieldmask.FieldMask) fieldmask.FieldMask {
	if a == nil || b == nil {
		return nil
	}
	out := make(fieldmask.FieldMask, len(a)+len(b))
	maps.Copy(out, a)
	for name, child := range b {
		if previous, ok := out[name]; ok {
			child = unionScope(previous, child)
		}
		out[name] = child
	}
	return out
}

// UnionMapScopes prepares a mask for go-aip's projection, which otherwise
// gives exact map keys precedence over the wildcard. The input is unchanged.
func UnionMapScopes(tree fieldmask.FieldMask, md protoreflect.MessageDescriptor) fieldmask.FieldMask {
	var result fieldmask.FieldMask
	for name, subtree := range tree {
		fd := md.Fields().ByName(protoreflect.Name(name))
		if subtree == nil || fd == nil || fd.IsList() || fd.Message() == nil {
			continue
		}
		if result == nil {
			result = maps.Clone(tree)
		}
		if fd.IsMap() {
			result[name] = unionMapEntries(subtree, fd.MapValue().Message())
		} else {
			result[name] = UnionMapScopes(subtree, fd.Message())
		}
	}
	if result == nil {
		return tree
	}
	return result
}

func unionMapEntries(tree fieldmask.FieldMask, md protoreflect.MessageDescriptor) fieldmask.FieldMask {
	if md == nil {
		return tree
	}
	wildcard, hasWildcard := tree["*"]
	result := make(fieldmask.FieldMask, len(tree))
	for key, subtree := range tree {
		if hasWildcard && key != "*" {
			subtree = unionScope(subtree, wildcard)
		}
		result[key] = UnionMapScopes(subtree, md)
	}
	return result
}

// RangeMessages visits the message instances directly held by a populated field.
func RangeMessages(fd protoreflect.FieldDescriptor, value protoreflect.Value, visit func(protoreflect.Message)) {
	if fd.Message() == nil {
		return
	}
	switch {
	case fd.IsList():
		items := value.List()
		for i := range items.Len() {
			visit(items.Get(i).Message())
		}
	case fd.IsMap():
		if fd.MapValue().Message() != nil {
			value.Map().Range(func(_ protoreflect.MapKey, value protoreflect.Value) bool {
				visit(value.Message())
				return true
			})
		}
	default:
		visit(value.Message())
	}
}

// HasPartialMap reports whether Buf needs a separate view of the selected map
// entries. Scalars and ordinary nested messages can be selected by its filter.
func HasPartialMap(msg protoreflect.Message, tree fieldmask.FieldMask) bool {
	return visitPartialMaps(msg, tree, false)
}

// PruneMaps removes unselected map entries from a private validation copy.
func PruneMaps(msg protoreflect.Message, tree fieldmask.FieldMask) {
	visitPartialMaps(msg, tree, true)
}

func visitPartialMaps(msg protoreflect.Message, tree fieldmask.FieldMask, prune bool) bool {
	if tree == nil {
		return false
	}
	found := false
	msg.Range(func(fd protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		sub := tree[string(fd.Name())]
		if sub == nil || fd.Message() == nil {
			return true
		}
		switch {
		case fd.IsMap():
			found = true
			if !prune {
				return false
			}
			value.Map().Range(func(key protoreflect.MapKey, item protoreflect.Value) bool {
				child, keep := MapKeyScope(sub, key)
				if !keep {
					value.Map().Clear(key)
				} else if fd.MapValue().Message() != nil {
					visitPartialMaps(item.Message(), child, true)
				}
				return true
			})
		case fd.IsList():
			// Resolved selections validate the entire list.
		default:
			found = visitPartialMaps(value.Message(), sub, prune) || found
		}
		return prune || !found
	})
	return found
}

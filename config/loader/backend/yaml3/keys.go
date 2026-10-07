// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/altessa-s/go-atlas/config/loader/backend"
)

// nullTag is the resolved tag of a YAML null.
const nullTag = "!!null"

var (
	nodeType                = reflect.TypeFor[yaml.Node]()
	unmarshalerType         = reflect.TypeFor[yaml.Unmarshaler]()
	obsoleteUnmarshalerType = reflect.TypeFor[interface {
		UnmarshalYAML(unmarshal func(any) error) error
	}]()

	errDuplicateKey = errors.New("yaml: duplicated struct key")
	errBadInline    = errors.New("yaml: ,inline needs a struct or map field")
)

// DecodeKeys parses the first YAML document and returns its root value; an
// empty document is a null value. Children are bound by decoding them with
// yaml.v3 into maps and slices of [yaml.Node] typed after the destination, so
// key resolution (aliases, !!binary and null keys, "0x10" kept as a string key
// but 16 as an int key, "yes" as a bool key) and "<<" merge precedence are
// yaml.v3's own.
func (b *Backend) DecodeKeys(reader io.Reader) (backend.KeyNode, error) {
	var doc yaml.Node
	if err := yaml.NewDecoder(reader).Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return &keyNode{node: &doc}, nil
}

// keyNode is a value of a YAML document.
type keyNode struct {
	node *yaml.Node
	// aliases lists the anchored nodes of the aliases followed to reach node,
	// so a value that contains itself is not bound forever.
	aliases *aliasChain
}

// aliasChain is a linked list of the anchored nodes of followed aliases.
type aliasChain struct {
	anchor *yaml.Node
	next   *aliasChain
}

// contains reports whether the chain holds anchor.
func (c *aliasChain) contains(anchor *yaml.Node) bool {
	for ; c != nil; c = c.next {
		if c.anchor == anchor {
			return true
		}
	}
	return false
}

// target follows document and alias indirections to the node holding the
// value. It returns nil for an empty document and false for an alias that
// refers to a node containing it, which yaml.v3 rejects as well.
func (k *keyNode) target() (*yaml.Node, *aliasChain, bool) {
	n, chain := k.node, k.aliases
	for n != nil {
		switch n.Kind {
		case yaml.DocumentNode:
			if len(n.Content) == 0 {
				return nil, chain, true
			}
			n = n.Content[0]
		case yaml.AliasNode:
			// Decoding copies alias nodes; their anchored targets are shared.
			if chain.contains(n.Alias) {
				return nil, chain, false
			}
			chain = &aliasChain{anchor: n.Alias, next: chain}
			n = n.Alias
		default:
			return n, chain, true
		}
	}
	return nil, chain, true
}

// IsNull reports whether the value is null, including an empty document.
func (k *keyNode) IsNull() bool {
	n, _, ok := k.target()
	return ok && (n == nil || n.Kind == 0 || n.Kind == yaml.ScalarNode && n.ShortTag() == nullTag)
}

// decode decodes the value's own level into out and returns the alias chain
// its children are reached through, or false when the value does not decode
// into out.
func (k *keyNode) decode(out any) (*aliasChain, bool) {
	n, chain, ok := k.target()
	if !ok || n == nil {
		return nil, false
	}
	return chain, n.Decode(out) == nil
}

// Fields binds a mapping to struct type t. Keys are decoded as yaml.v3 decodes
// struct keys — into a string, with "<<" merges excluding keys by their
// resolved value — and matched against t's fields by yaml.v3's rules: the yaml
// tag name or the lowercased field name, flattening only ",inline" fields.
func (k *keyNode) Fields(t reflect.Type) (map[int]backend.KeyNode, bool) {
	if customDecoded(t) {
		return nil, false
	}
	info, err := structKeysOf(t)
	if err != nil {
		return nil, false
	}
	var m map[string]yaml.Node
	chain, ok := k.decode(&m)
	if !ok {
		return nil, false
	}

	out := make(map[int]backend.KeyNode, len(m))
	for key, n := range m {
		node := &keyNode{node: &n, aliases: chain}
		if path, ok := info.fields[key]; ok {
			insertField(out, path, node)
			continue
		}
		if info.inlineMap >= 0 {
			im, _ := out[info.inlineMap].(*entriesNode)
			if im == nil {
				im = &entriesNode{entries: map[any]backend.KeyNode{}}
				out[info.inlineMap] = im
			}
			im.entries[key] = node
		}
	}
	return out, true
}

// Entries binds a mapping to map type t, decoding each key into t's key type.
func (k *keyNode) Entries(t reflect.Type) (map[any]backend.KeyNode, bool) {
	if customDecoded(t) {
		return nil, false
	}
	m := reflect.New(reflect.MapOf(t.Key(), nodeType))
	chain, ok := k.decode(m.Interface())
	if !ok {
		return nil, false
	}

	out := make(map[any]backend.KeyNode, m.Elem().Len())
	for it := m.Elem().MapRange(); it.Next(); {
		n, _ := reflect.TypeAssert[yaml.Node](it.Value())
		out[it.Key().Interface()] = &keyNode{node: &n, aliases: chain}
	}
	return out, true
}

// Elems binds a sequence to slice or array type t. Like yaml.v3, it drops a
// null element that t's element type cannot hold, shifting later elements.
func (k *keyNode) Elems(t reflect.Type) ([]backend.KeyNode, bool) {
	if customDecoded(t) {
		return nil, false
	}
	var s []yaml.Node
	chain, ok := k.decode(&s)
	if !ok {
		return nil, false
	}

	keepNull := nullable(t.Elem().Kind())
	out := make([]backend.KeyNode, 0, len(s))
	for i := range s {
		n := &keyNode{node: &s[i], aliases: chain}
		if !keepNull && n.IsNull() {
			continue
		}
		out = append(out, n)
	}
	return out, true
}

// nullable reports whether yaml.v3 stores a null into a value of kind k.
func nullable(k reflect.Kind) bool {
	switch k {
	case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice:
		return true
	default:
		return false
	}
}

// customDecoded reports whether yaml.v3 hands values of type t to t's own
// unmarshaler (or stores the raw node), so their keys bind to nothing the
// loader can see.
func customDecoded(t reflect.Type) bool {
	pt := reflect.PointerTo(t)
	return t == nodeType || pt.Implements(unmarshalerType) || pt.Implements(obsoleteUnmarshalerType)
}

// insertField stores node under the field index path, creating the nodes of
// the inline fields along it.
func insertField(out map[int]backend.KeyNode, path []int, node backend.KeyNode) {
	if len(path) == 1 {
		out[path[0]] = node
		return
	}
	in, _ := out[path[0]].(*fieldsNode)
	if in == nil {
		in = &fieldsNode{fields: map[int]backend.KeyNode{}}
		out[path[0]] = in
	}
	insertField(in.fields, path[1:], node)
}

// fieldsNode holds the fields of an inline struct, which yaml.v3 binds from
// the keys of the enclosing mapping.
type fieldsNode struct {
	fields map[int]backend.KeyNode
}

// IsNull reports false: an inline struct is never null.
func (n *fieldsNode) IsNull() bool { return false }

// Fields returns the inline struct's fields.
func (n *fieldsNode) Fields(reflect.Type) (map[int]backend.KeyNode, bool) { return n.fields, true }

// Entries reports false: an inline struct is not a map.
func (n *fieldsNode) Entries(reflect.Type) (map[any]backend.KeyNode, bool) { return nil, false }

// Elems reports false: an inline struct is not a sequence.
func (n *fieldsNode) Elems(reflect.Type) ([]backend.KeyNode, bool) { return nil, false }

// entriesNode holds the entries of an inline map, which yaml.v3 fills with the
// keys of the enclosing mapping that bind to no field.
type entriesNode struct {
	entries map[any]backend.KeyNode
}

// IsNull reports false: an inline map is never null.
func (n *entriesNode) IsNull() bool { return false }

// Fields reports false: an inline map is not a struct.
func (n *entriesNode) Fields(reflect.Type) (map[int]backend.KeyNode, bool) { return nil, false }

// Entries returns the inline map's entries.
func (n *entriesNode) Entries(reflect.Type) (map[any]backend.KeyNode, bool) { return n.entries, true }

// Elems reports false: an inline map is not a sequence.
func (n *entriesNode) Elems(reflect.Type) ([]backend.KeyNode, bool) { return nil, false }

// structKeys maps the keys of a struct type to field index paths the way
// yaml.v3's getStructInfo does.
type structKeys struct {
	fields    map[string][]int
	inlineMap int
}

// structKeysResult is a cached structKeysOf result.
type structKeysResult struct {
	keys *structKeys
	err  error
}

var structKeysCache sync.Map // map[reflect.Type]structKeysResult

// structKeysOf returns the keys of struct type t, cached per type.
func structKeysOf(t reflect.Type) (*structKeys, error) {
	if r, ok := structKeysCache.Load(t); ok {
		res, _ := r.(structKeysResult)
		return res.keys, res.err
	}
	keys, err := buildStructKeys(t, map[reflect.Type]bool{})
	structKeysCache.Store(t, structKeysResult{keys: keys, err: err})
	return keys, err
}

// buildStructKeys mirrors yaml.v3's getStructInfo: unexported fields other
// than embedded ones are skipped, "-" excludes a field, an untagged field binds
// to its lowercased name — embedded ones included — and only ",inline" fields
// are flattened. An inline struct that unmarshals itself contributes no keys.
func buildStructKeys(t reflect.Type, visiting map[reflect.Type]bool) (*structKeys, error) {
	if visiting[t] {
		return nil, errBadInline
	}
	visiting[t] = true
	defer delete(visiting, t)

	sk := &structKeys{fields: map[string][]int{}, inlineMap: -1}
	for i := range t.NumField() {
		sf := t.Field(i)
		if sf.PkgPath != "" && !sf.Anonymous {
			continue
		}
		tag := sf.Tag.Get("yaml")
		if tag == "" && !strings.Contains(string(sf.Tag), ":") {
			tag = string(sf.Tag)
		}
		if tag == "-" {
			continue
		}

		name, flags, _ := strings.Cut(tag, ",")
		inline := false
		for flag := range strings.SplitSeq(flags, ",") {
			inline = inline || flag == "inline"
		}

		if !inline {
			if name == "" {
				name = strings.ToLower(sf.Name)
			}
			if err := addKey(sk, name, []int{i}); err != nil {
				return nil, err
			}
			continue
		}

		if sf.Type.Kind() == reflect.Map {
			sk.inlineMap = i
			continue
		}
		ft := sf.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() != reflect.Struct {
			return nil, errBadInline
		}
		if reflect.PointerTo(ft).Implements(unmarshalerType) {
			continue
		}
		inner, err := buildStructKeys(ft, visiting)
		if err != nil {
			return nil, err
		}
		for key, path := range inner.fields {
			if err := addKey(sk, key, append([]int{i}, path...)); err != nil {
				return nil, err
			}
		}
	}
	return sk, nil
}

// addKey binds key to a field index path, rejecting a key bound twice as
// yaml.v3 does.
func addKey(sk *structKeys, key string, path []int) error {
	if _, dup := sk.fields[key]; dup {
		return errDuplicateKey
	}
	sk.fields[key] = path
	return nil
}

// Ensure Backend implements backend.KeyDecoder.
var _ backend.KeyDecoder = (*Backend)(nil)

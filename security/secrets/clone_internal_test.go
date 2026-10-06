// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"net/netip"
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
)

type (
	cloneInner struct {
		Tags []string
	}
	cloneNested struct {
		Name  string
		List  []string
		Attrs map[string]string
		Inner *cloneInner
		Any   any
	}
	cloneNode struct {
		Name string
		Next *cloneNode
	}
	cloneShared struct {
		A, B *cloneInner
	}
	cloneInterior struct {
		Field int
		Ref   *int
	}
	cloneViews struct {
		A, B []int
	}
	cloneKey struct {
		ID  int
		Ptr *cloneInner
	}
	selfCloner struct {
		Data   []byte
		cloned bool
	}
)

func (s selfCloner) Clone() selfCloner {
	return selfCloner{Data: append([]byte(nil), s.Data...), cloned: true}
}

// TestCloneSecret_MutationIsolation checks that mutating a copy never
// changes the original, for every reference shape copied by reflection.
func TestCloneSecret_MutationIsolation(t *testing.T) {
	t.Parallel()

	t.Run("map", func(t *testing.T) {
		t.Parallel()
		orig := map[string]string{"user": "u"}
		c := cloneSecret(orig)
		c["user"] = "changed"
		c["new"] = "x"
		require.Equal(t, map[string]string{"user": "u"}, orig)
	})

	t.Run("nested struct", func(t *testing.T) {
		t.Parallel()
		orig := cloneNested{
			Name: "n", List: []string{"a"}, Attrs: map[string]string{"k": "v"},
			Inner: &cloneInner{Tags: []string{"t"}}, Any: []byte("raw"),
		}
		c := cloneSecret(orig)
		c.List[0] = "changed"
		c.Attrs["k"] = "changed"
		c.Inner.Tags[0] = "changed"
		c.Any.([]byte)[0] = 'X'
		require.Equal(t, "a", orig.List[0])
		require.Equal(t, "v", orig.Attrs["k"])
		require.Equal(t, "t", orig.Inner.Tags[0])
		require.Equal(t, "raw", string(orig.Any.([]byte)))
	})

	t.Run("pointer", func(t *testing.T) {
		t.Parallel()
		orig := &cloneInner{Tags: []string{"t"}}
		c := cloneSecret(orig)
		require.NotSame(t, orig, c)
		c.Tags[0] = "changed"
		require.Equal(t, "t", orig.Tags[0])
	})

	t.Run("string slice", func(t *testing.T) {
		t.Parallel()
		orig := []string{"a", "b"}
		c := cloneSecret(orig)
		c[0] = "changed"
		require.Equal(t, []string{"a", "b"}, orig)
	})

	t.Run("any holding bytes", func(t *testing.T) {
		t.Parallel()
		var orig any = []byte("raw")
		c := cloneSecret(orig)
		c.([]byte)[0] = 'X'
		require.Equal(t, "raw", string(orig.([]byte)))
	})

	t.Run("any holding map", func(t *testing.T) {
		t.Parallel()
		var orig any = map[string]any{"k": []byte("v")}
		c := cloneSecret(orig)
		c.(map[string]any)["k"].([]byte)[0] = 'X'
		c.(map[string]any)["new"] = 1
		require.Equal(t, map[string]any{"k": []byte("v")}, orig)
	})

	t.Run("pointer keys", func(t *testing.T) {
		t.Parallel()
		key := &cloneInner{Tags: []string{"t"}}
		orig := map[*cloneInner]string{key: "v"}
		c := cloneSecret(orig)
		for k := range c {
			require.NotSame(t, key, k, "a copied pointer key is a new identity")
			k.Tags[0] = "changed"
		}
		require.Equal(t, "t", key.Tags[0])
	})

	t.Run("struct key with pointer", func(t *testing.T) {
		t.Parallel()
		inner := &cloneInner{Tags: []string{"t"}}
		orig := map[cloneKey]string{{ID: 1, Ptr: inner}: "v"}
		c := cloneSecret(orig)
		for k := range c {
			k.Ptr.Tags[0] = "changed"
		}
		require.Equal(t, "t", inner.Tags[0])
	})

	t.Run("Clone method", func(t *testing.T) {
		t.Parallel()
		orig := selfCloner{Data: []byte("d")}
		c := cloneSecret(orig)
		require.True(t, c.cloned, "a payload's own Clone method is used")
		c.Data[0] = 'X'
		require.Equal(t, "d", string(orig.Data))
	})

	t.Run("nil references", func(t *testing.T) {
		t.Parallel()
		var orig cloneNested
		c := cloneSecret(orig)
		require.Nil(t, c.List)
		require.Nil(t, c.Attrs)
		require.Nil(t, c.Inner)
		require.Nil(t, c.Any)
	})
}

// TestCloneSecret_FastPaths checks that string and []byte payloads, copied
// without reflection, give the reflection path's result.
func TestCloneSecret_FastPaths(t *testing.T) {
	t.Parallel()

	t.Run("string", func(t *testing.T) {
		t.Parallel()
		orig := string([]byte("payload"))
		c := cloneSecret(orig)
		require.Equal(t, orig, c)
		require.Equal(t, cloneReflect(orig), c)
		require.NotSame(t, unsafe.StringData(orig), unsafe.StringData(c), "the copy has its own bytes")
	})

	t.Run("bytes", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			orig []byte
		}{
			{name: "nil", orig: nil},
			{name: "empty", orig: []byte{}},
			{name: "payload", orig: []byte("payload")},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				c := cloneSecret(tc.orig)
				want := cloneReflect(tc.orig)
				require.Equal(t, want == nil, c == nil, "nil-ness matches the reflection path")
				require.Equal(t, want, c)
				require.Len(t, c, len(tc.orig))
				if len(c) > 0 {
					c[0] = 'X'
					require.Equal(t, byte('p'), tc.orig[0], "mutating the copy leaves the original intact")
				}
			})
		}
	})

	t.Run("other types take the reflection path", func(t *testing.T) {
		t.Parallel()
		var anyBytes any = []byte("raw")
		c := cloneSecret(anyBytes)
		c.([]byte)[0] = 'X'
		require.Equal(t, "raw", string(anyBytes.([]byte)))

		clonerBytes := cloneSecret(bytesCloner("raw"))
		require.Equal(t, bytesCloner("cloned"), clonerBytes, "a defined []byte type's Clone method is used")
		require.Equal(t, definedString("s"), cloneSecret(definedString("s")))
	})
}

// bytesCloner is a defined []byte payload with its own Clone method.
type bytesCloner []byte

func (bytesCloner) Clone() bytesCloner { return bytesCloner("cloned") }

// TestCloneSecret_CyclesAndAliases checks termination on cycles and that
// only identical references stay shared within a copy.
func TestCloneSecret_CyclesAndAliases(t *testing.T) {
	t.Parallel()

	t.Run("cyclic pointer", func(t *testing.T) {
		t.Parallel()
		orig := &cloneNode{Name: "a"}
		orig.Next = orig
		c := cloneSecret(orig)
		require.NotSame(t, orig, c)
		require.Same(t, c, c.Next, "the cycle is preserved in the copy")
	})

	t.Run("self-referential map", func(t *testing.T) {
		t.Parallel()
		orig := map[string]any{"name": "m"}
		orig["self"] = orig
		c := cloneSecret(orig)
		self := c["self"].(map[string]any)
		self["name"] = "changed"
		require.Equal(t, "changed", c["name"], "the cycle is preserved in the copy")
		require.Equal(t, "m", orig["name"])
	})

	t.Run("self-referential slice in any", func(t *testing.T) {
		t.Parallel()
		s := make([]any, 2)
		s[0] = s
		s[1] = "x"
		var orig any = s
		c := cloneSecret(orig).([]any)
		inner := c[0].([]any)
		inner[1] = "changed"
		require.Equal(t, "changed", c[1], "the cycle is preserved in the copy")
		require.Equal(t, "x", s[1])
	})

	t.Run("identical references stay shared", func(t *testing.T) {
		t.Parallel()
		inner := &cloneInner{Tags: []string{"t"}}
		c := cloneSecret(cloneShared{A: inner, B: inner})
		require.Same(t, c.A, c.B)
		require.NotSame(t, inner, c.A)
	})

	t.Run("overlapping views become independent", func(t *testing.T) {
		t.Parallel()
		b := []int{1, 2}
		c := cloneSecret(cloneViews{A: b, B: b[1:]})
		c.A[1] = 9
		require.Equal(t, 2, c.B[0], "overlapping views are copied independently")
		require.Equal(t, []int{1, 2}, b)
	})

	t.Run("interior pointer becomes independent", func(t *testing.T) {
		t.Parallel()
		orig := &cloneInterior{Field: 1}
		orig.Ref = &orig.Field
		c := cloneSecret(orig)
		*c.Ref = 5
		require.Equal(t, 1, c.Field, "interior pointers are copied independently")
		require.Equal(t, 1, orig.Field)
	})
}

type (
	namedNodePtr *cloneNode
	// privateCloner keeps its secret in an unexported field and copies it in
	// its own Clone method.
	privateCloner struct {
		secret []byte
	}
	clonerHolder struct {
		C privateCloner
		M map[string]privateCloner
	}
)

func (p privateCloner) Clone() privateCloner {
	return privateCloner{secret: append([]byte(nil), p.secret...)}
}

func (p privateCloner) set(b byte) { p.secret[0] = b }

// TestCloneSecret_DefinedPointerAndNestedCloners checks that defined pointer
// types keep their type, and that Clone methods are used wherever the value
// occurs, including behind an interface.
func TestCloneSecret_DefinedPointerAndNestedCloners(t *testing.T) {
	t.Parallel()

	t.Run("defined pointer in interface", func(t *testing.T) {
		t.Parallel()
		node := &cloneNode{Name: "a"}
		node.Next = node
		var orig any = namedNodePtr(node)
		c := cloneSecret(orig)
		p, ok := c.(namedNodePtr)
		require.True(t, ok, "the dynamic type must be kept")
		require.NotSame(t, (*cloneNode)(node), (*cloneNode)(p))
		require.Same(t, (*cloneNode)(p), p.Next, "the cycle is preserved")
	})

	t.Run("interface map keys", func(t *testing.T) {
		t.Parallel()
		node := &cloneNode{Name: "k"}
		orig := map[any]string{namedNodePtr(node): "v"}
		c := cloneSecret(orig)
		for k := range c {
			p, ok := k.(namedNodePtr)
			require.True(t, ok)
			p.Name = "changed"
		}
		require.Equal(t, "k", node.Name)
	})

	t.Run("cloner behind any", func(t *testing.T) {
		t.Parallel()
		var orig any = privateCloner{secret: []byte("s")}
		c := cloneSecret(orig)
		c.(privateCloner).set('X')
		require.Equal(t, "s", string(orig.(privateCloner).secret))
	})

	t.Run("nested cloners", func(t *testing.T) {
		t.Parallel()
		orig := clonerHolder{
			C: privateCloner{secret: []byte("s")},
			M: map[string]privateCloner{"k": {secret: []byte("m")}},
		}
		c := cloneSecret(orig)
		c.C.set('X')
		c.M["k"].set('Y')
		require.Equal(t, "s", string(orig.C.secret))
		require.Equal(t, "m", string(orig.M["k"].secret))
	})
}

type cloneablePayload struct {
	User    string
	Tags    []string
	Created time.Time
	Addr    netip.Addr
	ttl     time.Duration
	mu      sync.Mutex
}

type secretBytes struct{ raw []byte }

type withClone struct{ raw []byte }

func (w withClone) Clone() withClone { return withClone{raw: append([]byte(nil), w.raw...)} }

type node struct {
	Next  *node
	Value string
}

func TestCheckCloneable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		typ  reflect.Type
		bad  string // substring of the reported path; empty when cloneable
	}{
		{"string", reflect.TypeFor[string](), ""},
		{"bytes", reflect.TypeFor[[]byte](), ""},
		{"exported_fields_and_immutable_stdlib", reflect.TypeFor[cloneablePayload](), ""},
		{"recursive", reflect.TypeFor[*node](), ""},
		{"clone_method", reflect.TypeFor[withClone](), ""},
		{"map_of_clone_method", reflect.TypeFor[map[string]withClone](), ""},
		{"unexported_slice", reflect.TypeFor[secretBytes](), "secretBytes.raw"},
		{"nested_unexported_slice", reflect.TypeFor[map[string][]secretBytes](), ".raw"},
		{"exported_chan", reflect.TypeFor[struct{ C chan int }](), ".C (chan)"},
		{"func", reflect.TypeFor[func()](), "(func)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := checkCloneable(tc.typ)
			if tc.bad == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrUncloneablePayload)
			require.ErrorContains(t, err, tc.bad)
		})
	}
}

type innerWithChan struct {
	N int
	C chan int
}

// The value check terminates on self-referential slices and tells apart
// references that share an address but not a type.
func TestCheckCloneableValue_References(t *testing.T) {
	t.Parallel()

	cyclic := make([]any, 1)
	cyclic[0] = cyclic
	require.NoError(t, checkCloneableValue(reflect.ValueOf(&[]any{cyclic}).Elem()))

	p := &innerWithChan{C: make(chan int)}
	payload := any(struct {
		A *int
		B *innerWithChan
	}{&p.N, p})
	err := checkCloneableValue(reflect.ValueOf(&payload).Elem())
	require.ErrorIs(t, err, ErrUncloneablePayload)
	require.ErrorContains(t, err, ".B[elem].C (chan)")
}

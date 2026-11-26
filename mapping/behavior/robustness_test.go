// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/mapping/behavior"
)

const childEnv = "BEHAVIOR_ROBUSTNESS_CHILD"

type cyclicNode struct {
	Parent *cyclicNode
	Name   string `behavior:"output_only"`
}

type recursiveTree []recursiveTree

type treeHolder struct {
	Tree   recursiveTree
	Secret string `behavior:"input_only"`
}

// TestUnboundedRecursionTerminates runs each case in a subprocess with a small stack limit: an unbounded recursion is a fatal stack
// overflow that no recover can catch.
func TestUnboundedRecursionTerminates(t *testing.T) {
	t.Parallel()

	if c := os.Getenv(childEnv); c != "" {
		debug.SetMaxStack(4 << 20)
		os.Exit(runChild(c))
	}

	for _, name := range []string{"cyclic-clean", "recursive-collection-type"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestUnboundedRecursionTerminates$")
			cmd.Env = append(os.Environ(), childEnv+"="+name)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "child output: %.2000s", out)
		})
	}
}

func runChild(name string) int {
	switch name {
	case "cyclic-clean":
		a := &cyclicNode{Name: "x"}
		a.Parent = a
		_, err := behavior.Clean(*a, behavior.WithKinds(behavior.OutputOnly))
		if !errors.Is(err, behavior.ErrMaxDepthExceeded) {
			return 2
		}
	case "recursive-collection-type":
		h := treeHolder{Tree: recursiveTree{}, Secret: "s"}
		if err := behavior.StripResponse(&h); err != nil || h.Secret != "" {
			return 3
		}
	default:
		return 4
	}
	return 0
}

type credentials struct {
	Password string `behavior:"input_only"`
}

type account struct {
	credentials
}

type accountResponse struct {
	Account account
	Name    string
}

// TestStripResponseTraversesPrivateEmbeddedStruct: a field whose struct embeds only a private type still has its promoted input-only
// fields stripped.
func TestStripResponseTraversesPrivateEmbeddedStruct(t *testing.T) {
	t.Parallel()

	const marker = "s3cr3t-marker"
	r := accountResponse{Account: account{credentials: credentials{Password: marker}}, Name: "n"}
	require.NoError(t, behavior.StripResponse(&r))

	out, err := json.Marshal(r)
	require.NoError(t, err)
	require.NotContains(t, string(out), marker)
}

type privateBase struct {
	ID string `behavior:"identifier"`
}

type withPrivatePointer struct {
	*privateBase
	Name string
}

// TestCleanDoesNotTouchTheOriginalThroughPrivateEmbeddedPointer: the clone must not share a private embedded pointer with the source.
func TestCleanDoesNotTouchTheOriginalThroughPrivateEmbeddedPointer(t *testing.T) {
	t.Parallel()

	orig := withPrivatePointer{privateBase: &privateBase{ID: "id-1"}, Name: "n"}
	cp, err := behavior.Clean(orig, behavior.WithKinds(behavior.Identifier))
	require.NoError(t, err)

	require.Equal(t, "id-1", orig.ID, "the original keeps its identifier")
	require.Empty(t, cp.ID, "the copy is cleaned")
	require.Equal(t, "n", cp.Name)
}

type sharedLeaf struct{ V int }

type sharedHolder struct {
	A, B *sharedLeaf
	Note string `behavior:"output_only"`
}

// TestCleanKeepsSharedNodesShared: a node reachable twice is copied once, and independently of the source.
func TestCleanKeepsSharedNodesShared(t *testing.T) {
	t.Parallel()

	leaf := &sharedLeaf{V: 1}
	cp, err := behavior.Clean(sharedHolder{A: leaf, B: leaf, Note: "n"}, behavior.WithKinds(behavior.OutputOnly))
	require.NoError(t, err)
	require.Same(t, cp.A, cp.B)
	require.NotSame(t, leaf, cp.A)
	require.Empty(t, cp.Note)
}

type chainNode struct{ Next *chainNode }

type clearedSubtree struct {
	Secret *chainNode `behavior:"output_only"`
	Name   string
}

type embeddedWithChild struct{ Child struct{ X int } }

type flattened struct {
	embeddedWithChild
	Tags []string
}

// TestCleanDepthRulesMatchStrip: Clean accepts what Strip accepts. A deep acyclic recursive collection is a leaf of the walk, a
// subtree cleared whole is never descended, and a flattened embedded struct costs no depth.
func TestCleanDepthRulesMatchStrip(t *testing.T) {
	t.Parallel()

	t.Run("deep acyclic recursive collection", func(t *testing.T) {
		t.Parallel()
		deep := recursiveTree{}
		for range 5000 {
			deep = recursiveTree{deep}
		}
		in := treeHolder{Tree: deep, Secret: "s"}
		cp, err := behavior.Clean(in, behavior.WithKinds(behavior.InputOnly))
		require.NoError(t, err)
		require.Empty(t, cp.Secret)
		require.Equal(t, "s", in.Secret)
	})

	t.Run("cleared subtree beyond the depth budget", func(t *testing.T) {
		t.Parallel()
		head := &chainNode{}
		for range 100 {
			head = &chainNode{Next: head}
		}
		in := clearedSubtree{Secret: head, Name: "n"}
		require.NoError(t, behavior.Strip(new(in), behavior.WithKinds(behavior.OutputOnly), behavior.WithMaxDepth(3)))
		cp, err := behavior.Clean(in, behavior.WithKinds(behavior.OutputOnly), behavior.WithMaxDepth(3))
		require.NoError(t, err)
		require.Nil(t, cp.Secret)
		require.NotNil(t, in.Secret)
	})

	t.Run("flattened embedded struct", func(t *testing.T) {
		t.Parallel()
		in := flattened{Tags: []string{"a"}}
		opts := []behavior.Option{behavior.WithKinds(behavior.OutputOnly), behavior.WithMaxDepth(1)}
		require.NoError(t, behavior.Strip(new(in), opts...))
		_, err := behavior.Clean(in, opts...)
		require.NoError(t, err)
	})

	t.Run("real nesting still bounded", func(t *testing.T) {
		t.Parallel()
		head := &chainNode{}
		for range 100 {
			head = &chainNode{Next: head}
		}
		type holder struct{ Chain *chainNode }
		_, err := behavior.Clean(holder{Chain: head}, behavior.WithKinds(behavior.OutputOnly), behavior.WithMaxDepth(3))
		require.ErrorIs(t, err, behavior.ErrMaxDepthExceeded)
	})
}

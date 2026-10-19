// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/behavior"
)

// policyProbe reports the policy it was bound to.
type policyProbe struct {
	bound behavior.Policy
}

func (p *policyProbe) BindPolicy(pol behavior.Policy) behavior.Translator[behavior.Policy] {
	return &policyProbe{bound: pol}
}

func (p *policyProbe) Translate(context.Context, behavior.Object) (behavior.Policy, error) {
	return p.bound, nil
}

func TestNewBindsPolicy(t *testing.T) {
	t.Parallel()

	type doc struct {
		Name string
	}
	probe := &policyProbe{}
	got, err := behavior.New[behavior.Policy](probe, behavior.WithTagName("beh"),
		behavior.WithKinds(behavior.Immutable, behavior.InputOnly)).Translate(t.Context(), &doc{})
	require.NoError(t, err)
	require.Equal(t, "beh", got.TagName())
	require.Equal(t, []behavior.Kind{behavior.InputOnly, behavior.Immutable}, got.Kinds())
	require.True(t, got.Strips(behavior.Immutable))
	require.False(t, got.Strips(behavior.OutputOnly))
	require.Equal(t, behavior.Policy{}, probe.bound, "the translator passed in is left unchanged")

	got, err = behavior.New[behavior.Policy](probe).Translate(t.Context(), &doc{})
	require.NoError(t, err)
	require.Equal(t, behavior.Policy{}, got, "no options: the zero policy")
}

func TestPolicyZero(t *testing.T) {
	t.Parallel()

	var p behavior.Policy
	require.Equal(t, behavior.DefaultTagName, p.TagName())
	require.Nil(t, p.Kinds())
	require.False(t, p.Strips(behavior.Unspecified))
	require.False(t, p.Strips(behavior.Identifier))
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldtracker_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/mapping/fieldtracker"
)

func TestUnicodeFieldNames(t *testing.T) {
	t.Parallel()

	type Record struct {
		État       int
		Имя        string
		ÉtatActuel string
		UserID     int
	}
	before := Record{}
	after := Record{État: 1, Имя: "Ada", ÉtatActuel: "active", UserID: 2}
	require.ElementsMatch(t, []string{"état", "имя", "état_actuel", "user_i_d"}, fieldtracker.GetChangedFields(before, after))
}

func TestFlattenedEmbeddedChanges(t *testing.T) {
	t.Parallel()

	type Base struct {
		Name string `json:"name"`
	}
	type Record struct{ *Base }
	for _, tc := range []struct {
		name   string
		before Record
		after  Record
		want   []string
	}{
		{name: "added", after: Record{&Base{}}, want: []string{"base"}},
		{name: "removed", before: Record{&Base{Name: "a"}}, want: []string{"base"}},
		{name: "both nil"},
		{name: "changed", before: Record{&Base{Name: "a"}}, after: Record{&Base{Name: "b"}}, want: []string{"name"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.ElementsMatch(t, tc.want, fieldtracker.GetChangedFields(tc.before, tc.after))
		})
	}
	t.Run("opaque exported field", func(t *testing.T) {
		t.Parallel()
		type Event struct{ time.Time }
		require.Equal(t, []string{"time"}, fieldtracker.GetChangedFields(Event{time.Unix(1, 0)}, Event{time.Unix(2, 0)}))
	})
	t.Run("opaque private field", func(t *testing.T) {
		t.Parallel()
		type internalState struct{ value int }
		type Event struct{ internalState }
		require.NotPanics(t, func() {
			require.Empty(t, fieldtracker.GetChangedFields(Event{internalState{1}}, Event{internalState{2}}))
		})
	})
}

func TestIgnoreCollectionSubtrees(t *testing.T) {
	t.Parallel()

	type Item struct {
		Name string `json:"name"`
	}
	type Record struct {
		Items []Item           `json:"items"`
		Array [2]Item          `json:"array"`
		ByID  map[string]*Item `json:"by_id"`
		Other string           `json:"other"`
	}
	before := Record{
		Items: []Item{{Name: "a"}, {Name: "b"}},
		Array: [2]Item{{Name: "a"}, {Name: "b"}},
		ByID:  map[string]*Item{"a": {Name: "a"}, "b": {Name: "b"}},
	}
	after := Record{
		Items: []Item{{Name: "changed"}, {Name: "changed"}},
		Array: [2]Item{{Name: "changed"}, {Name: "changed"}},
		ByID:  map[string]*Item{"a": {Name: "changed"}, "b": {Name: "changed"}},
		Other: "changed",
	}
	tr := fieldtracker.NewTracker(fieldtracker.WithIgnoreFields("items[0]", "array[0]", "by_id[a]"))
	want := []string{"items[1].name", "array[1].name", "by_id[b].name", "other"}
	require.ElementsMatch(t, want, tr.GetChangedFields(before, after))
	require.ElementsMatch(t, want, tr.GetChangedFields(after, before))
}

func TestEmbeddedFieldTags(t *testing.T) {
	t.Parallel()

	type Base struct {
		Name string `json:"name"`
	}
	type Ignored struct {
		Base `json:"-"`
	}
	type Named struct {
		*Base `json:"base"`
	}
	type Custom struct {
		Base `db:"details"`
	}

	t.Run("ignored", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, fieldtracker.GetChangedFields(Ignored{Base{Name: "a"}}, Ignored{Base{Name: "b"}}))
	})
	t.Run("nested ignored", func(t *testing.T) {
		t.Parallel()
		type Record struct{ Value Ignored }
		require.Empty(t, fieldtracker.GetChangedFields(Record{Ignored{Base{Name: "a"}}}, Record{Ignored{Base{Name: "b"}}}))
	})
	t.Run("named", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []string{"base.name"}, fieldtracker.GetChangedFields(Named{&Base{Name: "a"}}, Named{&Base{Name: "b"}}))
	})
	t.Run("named nil", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []string{"base"}, fieldtracker.GetChangedFields(Named{}, Named{&Base{Name: "b"}}))
	})
	t.Run("custom tag", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []string{"details.name"}, fieldtracker.GetChangedFields(Custom{Base{Name: "a"}}, Custom{Base{Name: "b"}},
			fieldtracker.WithTagName("db")))
	})
}

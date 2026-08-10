// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldtracker_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/fieldtracker"
)

// record is the shape the targets diff: scalars, a pointer that can be nil, a
// nested struct, and a slice — the four kinds of field a reflective comparison
// gets wrong in different ways.
type record struct {
	Name    string `json:"name"`
	Age     int    `json:"age"`
	Active  bool   `json:"active"`
	Comment *string
	Nested  struct {
		City string `json:"city"`
	}
	Tags []string
}

// FuzzUnchangedRecordsReportNothing pins the half of the contract that decides
// whether a write happens at all.
//
// Change tracking drives partial updates: the reported fields become the `$set`
// document. A spurious field is a write that clobbers a value someone else
// changed concurrently, and it is invisible in review because the update
// "looks" targeted.
func FuzzUnchangedRecordsReportNothing(f *testing.F) {
	f.Add("alice", 30, true, "note", "Berlin", "a,b")
	f.Add("", 0, false, "", "", "")
	f.Add("bob", -1, true, "", "x", "")

	f.Fuzz(func(t *testing.T, name string, age int, active bool, comment, city, tags string) {
		value := buildRecord(name, age, active, comment, city, tags)

		require.Empty(t, fieldtracker.GetChangedFields(value, value),
			"comparing a value with itself reported changes")
	})
}

// FuzzEveryDifferenceIsReported is the other half: a field that changed must
// appear.
//
// A missed field is the more expensive direction — the update silently drops
// part of the caller's intent, and the record stays half-written with no error
// anywhere. The comparison walks nested structs and slices by reflection, so a
// difference buried one level down is exactly what a shallow fast path misses.
func FuzzEveryDifferenceIsReported(f *testing.F) {
	f.Add("alice", "bob", 30, 31, true, false, "Berlin", "Munich")
	f.Add("", "", 0, 0, true, true, "", "")
	f.Add("x", "x", 1, 1, false, false, "a", "b")

	f.Fuzz(func(t *testing.T, nameA, nameB string, ageA, ageB int, activeA, activeB bool, cityA, cityB string) {
		before := buildRecord(nameA, ageA, activeA, "note", cityA, "a")
		after := buildRecord(nameB, ageB, activeB, "note", cityB, "a")

		changed := fieldtracker.GetChangedFields(before, after)

		if nameA != nameB {
			require.True(t, containsField(changed, "name"), "a changed name went unreported: %v", changed)
		}
		if ageA != ageB {
			require.True(t, containsField(changed, "age"), "a changed age went unreported: %v", changed)
		}
		if activeA != activeB {
			require.True(t, containsField(changed, "active"), "a changed flag went unreported: %v", changed)
		}
		if cityA != cityB {
			require.True(t, containsField(changed, "city"), "a changed nested field went unreported: %v", changed)
		}
	})
}

// FuzzComparisonIsSymmetricInSize pins that the answer does not depend on which
// value is called "before".
//
// The set of differing fields is a property of the pair, not of the argument
// order. A tracker that reports more fields one way round than the other is one
// whose nil-handling or slice comparison is directional — and half the callers
// would get the short answer.
func FuzzComparisonIsSymmetricInSize(f *testing.F) {
	f.Add("alice", "bob", "note", "", "a,b", "a")
	f.Add("", "", "", "", "", "")

	f.Fuzz(func(t *testing.T, nameA, nameB, commentA, commentB, tagsA, tagsB string) {
		before := buildRecord(nameA, 1, true, commentA, "Berlin", tagsA)
		after := buildRecord(nameB, 1, true, commentB, "Berlin", tagsB)

		forward := fieldtracker.GetChangedFields(before, after)
		backward := fieldtracker.GetChangedFields(after, before)

		slices.Sort(forward)
		slices.Sort(backward)
		require.Equal(t, forward, backward,
			"the reported change set depends on argument order")
	})
}

// buildRecord assembles a record from flat fuzzer inputs.
func buildRecord(name string, age int, active bool, comment, city, tags string) record {
	var r record
	r.Name = name
	r.Age = age
	r.Active = active
	if comment != "" {
		r.Comment = &comment
	}
	r.Nested.City = city
	if tags != "" {
		r.Tags = splitTags(tags)
	}
	return r
}

// splitTags turns a comma-separated seed into a slice without pulling in
// strings.Split's empty-element behavior, which would make "" and "," differ
// for reasons that are about the fixture rather than the tracker.
func splitTags(tags string) []string {
	var out []string
	for _, tag := range []byte(tags) {
		if tag != ',' {
			out = append(out, string(tag))
		}
	}
	return out
}

// containsField reports whether the change set names a field, matching either
// the bare name or a dotted path ending in it.
func containsField(changed []string, name string) bool {
	return slices.ContainsFunc(changed, func(f string) bool {
		return f == name || len(f) > len(name) && f[len(f)-len(name)-1:] == "."+name
	})
}

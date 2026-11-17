// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fetchEvents returns events named prefix+i created at base+offsets[i].
func fetchEvents(prefix string, base time.Time, offsets ...int) []event {
	out := make([]event, len(offsets))
	for i, off := range offsets {
		out[i] = event{Id: prefix + strconv.Itoa(i), CreatedAt: base.Add(time.Duration(off) * time.Millisecond)}
	}
	return out
}

func ids(events []event) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		out[i] = ev.Id
	}
	return out
}

func TestMergeFetchBatches(t *testing.T) {
	t.Parallel()
	base := time.Unix(1_700_000_000, 0).UTC()
	for _, tc := range []struct {
		name  string
		a, b  []event
		limit int
		want  []string
	}{
		{"both empty", nil, nil, 10, []string{}},
		{"only pending", fetchEvents("p", base, 1, 2), nil, 10, []string{"p0", "p1"}},
		{"only failed", nil, fetchEvents("f", base, 1, 2), 10, []string{"f0", "f1"}},
		{"interleaved", fetchEvents("p", base, 1, 4, 5), fetchEvents("f", base, 2, 3, 6), 10, []string{"p0", "f0", "f1", "p1", "p2", "f2"}},
		{"cut at limit", fetchEvents("p", base, 1, 4, 5), fetchEvents("f", base, 2, 3, 6), 3, []string{"p0", "f0", "f1"}},
		{"tie keeps pending first", fetchEvents("p", base, 1), fetchEvents("f", base, 1), 10, []string{"p0", "f0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, ids(mergeFetchBatches(tc.a, tc.b, tc.limit)))
		})
	}
}

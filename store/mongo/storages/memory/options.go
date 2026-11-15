// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

//go:generate go tool optgen generate

import (
	schedtask "github.com/altessa-s/go-horai/task"
)

// options configures Storage creation.
type options struct {
	// scheduler registers the cleanup task; it takes effect only together
	// with cleanupSchedule.
	scheduler schedtask.Registrar `optgen:"notnil"`
	// cleanupSchedule is the cron schedule of the cleanup task.
	cleanupSchedule string
	// maxEntries caps the number of stored cursors. Every page with a successor
	// stores one, so without a cap a client paging in a loop grows the map
	// without bound. At the cap the oldest cursors are evicted and resolve to
	// mongo.ErrCursorNotFound. Values below 1 are ignored.
	maxEntries int `optcheck:"min=1" optgen:"default=DefaultMaxEntries,onfail=skip"`
	// maxEntriesPerSubject caps the number of stored cursors bound to one
	// subject (mongo.WithListCursorSubject), so a single principal paging in a
	// loop evicts its own oldest cursors instead of everyone's. Cursors without
	// a subject are held only to maxEntries. 0 disables the cap; negative
	// values are ignored. When the option is not given, the cap is a tenth of
	// maxEntries, at least 1 and at most DefaultMaxEntriesPerSubject, so it
	// follows a lowered maxEntries. A value above maxEntries is the same as
	// maxEntries.
	maxEntriesPerSubject int `optcheck:"min=0" optgen:"onfail=skip,enables=maxEntriesPerSubjectSet"`
	// maxEntriesPerSubjectSet is set by WithMaxEntriesPerSubject; New derives
	// the per-subject cap from maxEntries while it is false.
	maxEntriesPerSubjectSet bool `opt:"-"`
}

// subjectCap returns the effective per-subject cap: the configured one, or a
// tenth of maxEntries when none was given, never above maxEntries.
func (o *options) subjectCap() int {
	if !o.maxEntriesPerSubjectSet {
		return min(max(1, o.maxEntries/subjectCapDivisor), DefaultMaxEntriesPerSubject)
	}
	return min(o.maxEntriesPerSubject, o.maxEntries)
}

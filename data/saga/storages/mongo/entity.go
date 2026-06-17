// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"time"

	"github.com/altessa-s/go-atlas/data/saga"
)

// stepRecord is the internal MongoDB representation of a [saga.StepRecord].
// Timestamps are stored as Unix seconds (0 = zero time) to avoid MongoDB's
// awkward encoding of the time.Time zero value and to keep range queries cheap.
type stepRecord struct {
	Name       string `bson:"name"`
	Stage      int    `bson:"stage"`
	Status     string `bson:"status"`
	Attempts   int    `bson:"attempts"`
	Error      string `bson:"error,omitempty"`
	StartedAt  int64  `bson:"started_at,omitempty"`
	FinishedAt int64  `bson:"finished_at,omitempty"`
}

// instance is the internal MongoDB document representation of a [saga.Instance].
// Instance-level timestamps use Unix seconds; deadline is 0 when unset (which
// excludes the instance from the deadline branch of the recovery scan).
type instance struct {
	Id         string       `bson:"_id"`
	Definition string       `bson:"definition"`
	Status     string       `bson:"status"`
	Stage      int          `bson:"stage"`
	Data       []byte       `bson:"data,omitempty"`
	Steps      []stepRecord `bson:"steps,omitempty"`
	CreatedAt  int64        `bson:"created_at"`
	UpdatedAt  int64        `bson:"updated_at"`
	Deadline   int64        `bson:"deadline,omitempty"`
	Version    int64        `bson:"version"`
	LastError  string       `bson:"last_error,omitempty"`
}

// toUnix converts a time to Unix seconds, mapping the zero time to 0.
func toUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// fromUnix converts Unix seconds back to a UTC time, mapping 0 to the zero time.
func fromUnix(sec int64) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0).UTC()
}

// toDocument converts a public [saga.Instance] into its MongoDB document.
func toDocument(inst *saga.Instance) instance {
	doc := instance{
		Id:         inst.ID,
		Definition: inst.Definition,
		Status:     string(inst.Status),
		Stage:      inst.Stage,
		Data:       inst.Data,
		CreatedAt:  toUnix(inst.CreatedAt),
		UpdatedAt:  toUnix(inst.UpdatedAt),
		Deadline:   toUnix(inst.Deadline),
		Version:    inst.Version,
		LastError:  inst.LastError,
	}
	if len(inst.Steps) > 0 {
		doc.Steps = make([]stepRecord, len(inst.Steps))
		for i, st := range inst.Steps {
			doc.Steps[i] = stepRecord{
				Name:       st.Name,
				Stage:      st.Stage,
				Status:     string(st.Status),
				Attempts:   st.Attempts,
				Error:      st.Error,
				StartedAt:  toUnix(st.StartedAt),
				FinishedAt: toUnix(st.FinishedAt),
			}
		}
	}
	return doc
}

// fromDocument converts a MongoDB document into a public [saga.Instance].
func fromDocument(doc *instance) *saga.Instance {
	inst := &saga.Instance{
		ID:         doc.Id,
		Definition: doc.Definition,
		Status:     saga.Status(doc.Status),
		Stage:      doc.Stage,
		Data:       doc.Data,
		CreatedAt:  fromUnix(doc.CreatedAt),
		UpdatedAt:  fromUnix(doc.UpdatedAt),
		Deadline:   fromUnix(doc.Deadline),
		Version:    doc.Version,
		LastError:  doc.LastError,
	}
	if len(doc.Steps) > 0 {
		inst.Steps = make([]saga.StepRecord, len(doc.Steps))
		for i, st := range doc.Steps {
			inst.Steps[i] = saga.StepRecord{
				Name:       st.Name,
				Stage:      st.Stage,
				Status:     saga.StepStatus(st.Status),
				Attempts:   st.Attempts,
				Error:      st.Error,
				StartedAt:  fromUnix(st.StartedAt),
				FinishedAt: fromUnix(st.FinishedAt),
			}
		}
	}
	return inst
}

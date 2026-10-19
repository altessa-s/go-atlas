// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"cmp"
	"reflect"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/retryqueue"
)

// Document field names used in filters, updates and indexes.
const (
	fID          = "_id"
	fSince       = "since"
	fAttempts    = "attempts"
	fNextAttempt = "nextAttemptAt"
	fState       = "state"
	fExpiredAt   = "expiredAt"
	fResolvedAt  = "resolvedAt"
	fGeneration  = "generation"
)

// reservedFields are the fields the store owns; the payload and the flag
// fields must not use them.
var reservedFields = []string{fID, fSince, fAttempts, fNextAttempt, fState, fExpiredAt, fResolvedAt, fGeneration}

// itemDoc is the BSON representation of an item. The payload is inlined, so
// its fields sit next to the store's. The flag fields have configurable names
// and are read and written outside this struct.
type itemDoc[P any] struct {
	Key           string    `bson:"_id"`
	Since         time.Time `bson:"since"`
	Attempts      int       `bson:"attempts"`
	NextAttemptAt time.Time `bson:"nextAttemptAt"`
	State         string    `bson:"state"`
	// The driver honors omitempty (via time.Time.IsZero), not omitzero. An
	// absent resolvedAt matters: the TTL index purges any resolved document
	// that has one, so a stored zero date would purge it at once.
	ExpiredAt  time.Time `bson:"expiredAt,omitempty"`
	ResolvedAt time.Time `bson:"resolvedAt,omitempty"`
	// Generation decodes as 0 from a document written without it.
	Generation int64 `bson:"generation"`
	Payload    P     `bson:",inline"`
}

// freshDoc returns item as the document Mark inserts and Reopen replaces the
// stored one with: pending, no attempts, no expiry, resolution or flag.
func freshDoc[P any](item retryqueue.Item[P]) itemDoc[P] {
	return itemDoc[P]{
		Key:           item.Key,
		Since:         utc(item.Since),
		NextAttemptAt: utc(item.NextAttemptAt),
		State:         string(retryqueue.StatePending),
		Generation:    item.Generation,
		Payload:       item.Payload,
	}
}

// toItem maps d to an item, reading the flag fields from raw, the same
// document undecoded.
func (d itemDoc[P]) toItem(raw bson.Raw, flagField, reasonField string) retryqueue.Item[P] {
	it := retryqueue.Item[P]{
		Key:           d.Key,
		Generation:    d.Generation,
		State:         retryqueue.State(d.State),
		Since:         d.Since,
		Attempts:      d.Attempts,
		NextAttemptAt: d.NextAttemptAt,
		ExpiredAt:     d.ExpiredAt,
		ResolvedAt:    d.ResolvedAt,
		Payload:       d.Payload,
	}
	if v, err := raw.LookupErr(flagField); err == nil {
		it.Flagged, _ = v.BooleanOK()
	}
	if v, err := raw.LookupErr(reasonField); err == nil {
		it.FlagReason, _ = v.StringValueOK()
	}
	return it
}

// utc normalizes t to UTC, keeping the zero time zero so omitempty drops it.
func utc(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	return t.UTC()
}

// genEq matches an item at generation g; generation 0 also matches a document
// written without the field.
func genEq(g int64) any {
	if g == 0 {
		return bson.M{"$in": bson.A{int64(0), nil}}
	}
	return g
}

// payloadFields returns the top-level document fields a payload of type t
// encodes, following the driver's struct tag rules: unexported fields and
// fields tagged "-" are skipped, the key defaults to the lowercased field name,
// and inlined structs contribute their own fields. ok is false unless t is a
// struct or a struct pointer without an inlined map anywhere: a map's keys are
// known only at run time, and an inlined map would also collect the store's
// flag fields on decode, so its fields could not be kept apart from the
// store's.
func payloadFields(t reflect.Type) (fields []string, ok bool) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, false
	}
	for sf := range t.Fields() {
		if !sf.IsExported() {
			continue
		}
		name, inline, skip := parseTag(sf)
		switch {
		case skip:
		case inline:
			inner, ok := payloadFields(sf.Type)
			if !ok {
				return nil, false
			}
			fields = append(fields, inner...)
		default:
			fields = append(fields, name)
		}
	}
	return fields, true
}

// parseTag reads the document key and the inline and skip options of sf as the
// driver's default struct tag parser does.
func parseTag(sf reflect.StructField) (name string, inline, skip bool) {
	tag, ok := sf.Tag.Lookup("bson")
	if !ok && !strings.Contains(string(sf.Tag), ":") && len(sf.Tag) > 0 {
		tag = string(sf.Tag)
	}
	if tag == "-" {
		return "", false, true
	}
	key, rest, _ := strings.Cut(tag, ",")
	name = cmp.Or(key, strings.ToLower(sf.Name))
	inline = key == "inline" // the driver matches options against the key too
	for part := range strings.SplitSeq(rest, ",") {
		inline = inline || part == "inline"
	}
	return name, inline, false
}

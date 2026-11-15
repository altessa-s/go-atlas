// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/reliability/retryqueue/storages/storagetest"

	retrymongo "github.com/altessa-s/go-atlas/reliability/retryqueue/storages/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// offlineColl returns a collection of a client that never connects: New
// performs no I/O, so its validation needs no server.
func offlineColl(t testing.TB) *mongo.Collection {
	t.Helper()
	client, err := mongo.Connect(mongoOptions.Client().ApplyURI("mongodb://127.0.0.1:1"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	return client.Database("x").Collection(collName)
}

func TestNewRejectsNilCollection(t *testing.T) {
	t.Parallel()

	s, err := retrymongo.New[storagetest.Payload](nil)
	require.Error(t, err)
	require.Nil(t, s)
}

func TestNewRejectsInvalidFlagFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts []retrymongo.Option
	}{
		{"reserved flag field", []retrymongo.Option{retrymongo.WithFlagField("state")}},
		{"reserved reason field", []retrymongo.Option{retrymongo.WithFlagReasonField("_id")}},
		{"shared name", []retrymongo.Option{retrymongo.WithFlagField("f"), retrymongo.WithFlagReasonField("f")}},
		{"operator", []retrymongo.Option{retrymongo.WithFlagField("$flag")}},
		{"dotted path", []retrymongo.Option{retrymongo.WithFlagReasonField("flag.reason")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, err := retrymongo.New[storagetest.Payload](offlineColl(t), tt.opts...)
			require.ErrorIs(t, err, retrymongo.ErrInvalidFlagField)
			require.Nil(t, s)
		})
	}
}

func TestNewAcceptsDefaultsAndCustomFlagFields(t *testing.T) {
	t.Parallel()

	for _, opts := range [][]retrymongo.Option{
		nil,
		{retrymongo.WithFlagField("needsManualRate"), retrymongo.WithFlagReasonField("manualRateReason")},
		{retrymongo.WithFlagField(""), retrymongo.WithFlagReasonField("  ")}, // ignored: defaults stay
	} {
		s, err := retrymongo.New[storagetest.Payload](offlineColl(t), opts...)
		require.NoError(t, err)
		require.NotNil(t, s)
	}
}

// Payload shapes for TestNewValidatesPayload.
type (
	clashTagged struct {
		State string `bson:"state"`
	}
	clashDefaultName struct {
		Attempts int // encodes as "attempts"
	}
	clashFlag struct {
		Why string `bson:"flagReason"`
	}
	clashInline struct {
		Inner clashTagged `bson:",inline"`
	}
	okSkipped struct {
		State  string `bson:"-"`
		since  string //nolint:unused // Unexported fields are not encoded.
		Amount int64  `bson:"amount,omitempty"`
	}
	okInline struct {
		Inner storagetest.Payload `bson:",inline"`
		Extra string              `bson:"extra"`
	}
	inlineMap struct {
		Note  string         `bson:"note"`
		Extra map[string]any `bson:",inline"`
	}
	nestedInlineMap struct {
		Inner inlineMap `bson:",inline"`
	}
	namedMap struct {
		Labels map[string]string `bson:"labels"` // a sub-document, not inlined
	}
)

// newErr returns the error of New for payload type P.
func newErr[P any](t testing.TB, opts ...retrymongo.Option) func() error {
	coll := offlineColl(t)
	return func() error {
		_, err := retrymongo.New[P](coll, opts...)
		return err
	}
}

func TestNewValidatesPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func() error
		valid bool
	}{
		{"store field by tag", newErr[clashTagged](t), false},
		{"store field by default name", newErr[clashDefaultName](t), false},
		{"default flag field", newErr[clashFlag](t), false},
		{"custom flag field frees the default name", newErr[clashFlag](t, retrymongo.WithFlagReasonField("reason")), true},
		{"nested inline", newErr[clashInline](t), false},
		{"not a document", newErr[string](t), false},
		{"int-keyed map", newErr[map[int]string](t), false},
		{"string-keyed map", newErr[map[string]string](t), false},
		{"any map", newErr[map[string]any](t), false},
		{"inline map", newErr[inlineMap](t), false},
		{"nested inline map", newErr[nestedInlineMap](t), false},
		{"map field", newErr[namedMap](t), true},
		{"skipped and unexported fields", newErr[okSkipped](t), true},
		{"inline struct", newErr[okInline](t), true},
		{"struct pointer", newErr[*storagetest.Payload](t), true},
		{"empty struct", newErr[struct{}](t), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.build()
			if tt.valid {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, retrymongo.ErrInvalidPayload)
		})
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storedoc_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/store/mongo/storedoc"
)

func TestClassify(t *testing.T) {
	t.Parallel()
	raw, err := bson.Marshal(bson.M{"n": "not a number"})
	require.NoError(t, err)
	var doc struct {
		N int `bson:"n"`
	}
	decodeErr := bson.Unmarshal(raw, &doc)
	require.Error(t, decodeErr)
	category := errors.New("corrupt record")
	transport := errors.New("connection reset")
	tests := []struct {
		name    string
		err     error
		corrupt bool
	}{
		{"nil", nil, false},
		{"decode", decodeErr, true},
		{"wrapped decode", fmt.Errorf("read: %w", decodeErr), true},
		{"value decoder", bson.ValueDecoderError{Name: "x"}, true},
		{"already classified", fmt.Errorf("%w: bad document", category), true},
		{"missing document", mongo.ErrNoDocuments, false},
		{"transport", transport, false},
		{"canceled", context.Canceled, false},
		{"query", fmt.Errorf("query: %w", mongo.CommandError{Code: 2}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := storedoc.Classify(tt.err, category)
			if tt.corrupt {
				require.ErrorIs(t, got, category)
				require.Same(t, got, storedoc.Classify(got, category))
			} else {
				require.NotErrorIs(t, got, category)
				require.Equal(t, tt.err, got)
			}
			if value, ok := errors.AsType[bson.ValueDecoderError](tt.err); ok {
				var actual bson.ValueDecoderError
				require.ErrorAs(t, got, &actual)
				require.Equal(t, value, actual)
			} else if tt.err != nil {
				require.ErrorIs(t, got, tt.err)
			}
			if original, ok := errors.AsType[*bson.DecodeError](tt.err); ok {
				var actual *bson.DecodeError
				require.ErrorAs(t, got, &actual)
				require.Same(t, original, actual)
			}
		})
	}
	t.Run("nil category", func(t *testing.T) {
		t.Parallel()
		require.Same(t, decodeErr, storedoc.Classify(decodeErr, nil))
	})
}

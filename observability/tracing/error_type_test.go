// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"
)

func TestRecordErrorConcreteType(t *testing.T) {
	t.Parallel()
	a := &captureAdapter{MultiAdapter: adapters.NewMultiAdapter()}
	tr := tracing.New(tracing.WithAdapter(a), tracing.WithSampler(sampler.AlwaysOn()))
	_, span := tr.Recorder("test").Start(t.Context(), "parse")
	_, err := strconv.Atoi("invalid")
	require.Error(t, err)
	span.RecordError(err)
	span.End()
	require.Len(t, a.spans, 1)
	require.Len(t, a.spans[0].Events, 1)
	require.Contains(t, a.spans[0].Events[0].Attributes, adapters.Attribute{Key: "exception.type", Value: "*strconv.NumError"})
}

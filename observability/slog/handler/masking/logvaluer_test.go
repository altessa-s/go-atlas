// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking_test

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/slog/handler/masking"
)

type secretLogValue struct{}

func (secretLogValue) LogValue() slog.Value {
	return slog.GroupValue(slog.String("password", "secret-value"), slog.String("user", "alice"))
}

type pointerLogValue struct{}

func (*pointerLogValue) LogValue() slog.Value { return secretLogValue{}.LogValue() }

type scalarLogValue struct{}

func (scalarLogValue) LogValue() slog.Value { return slog.StringValue("secret-value") }

type chainLogValue struct{}

func (chainLogValue) LogValue() slog.Value { return slog.AnyValue(secretLogValue{}) }

type cycleLogValue struct{}

func (cycleLogValue) LogValue() slog.Value { return slog.AnyValue(cycleLogValue{}) }

type groupCycleLogValue struct{}

func (groupCycleLogValue) LogValue() slog.Value {
	return slog.GroupValue(slog.Any("next", groupCycleLogValue{}))
}

type mapCycleLogValue struct{}

func (mapCycleLogValue) LogValue() slog.Value {
	return slog.AnyValue(map[string]any{"next": mapCycleLogValue{}})
}

func TestHandler_LogValuerMasksResolvedValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		attr slog.Attr
	}{
		{"group", slog.Any("payload", secretLogValue{})},
		{"pointer", slog.Any("payload", &pointerLogValue{})},
		{"chain", slog.Any("payload", chainLogValue{})},
		{"scalar", slog.Any("password", scalarLogValue{})},
		{"struct", slog.Any("payload", struct{ Value secretLogValue }{})},
		{"pointer struct", slog.Any("payload", struct{ Value *pointerLogValue }{&pointerLogValue{}})},
		{"map", slog.Any("payload", map[string]any{"value": secretLogValue{}})},
		{"slice", slog.Any("payload", []secretLogValue{{}})},
		{"group member", slog.Group("payload", slog.Any("value", secretLogValue{}))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, bound := range []bool{false, true} {
				var out bytes.Buffer
				h := masking.NewHandler(slog.NewJSONHandler(&out, nil), masking.WithField("password", masking.FullMask()))
				logger := slog.New(h).WithGroup("request")
				if bound {
					logger = logger.With(tc.attr)
					logger.InfoContext(t.Context(), "test")
				} else {
					logger.LogAttrs(t.Context(), slog.LevelInfo, "test", tc.attr)
				}
				require.NotContains(t, out.String(), "secret-value")
				require.Contains(t, out.String(), "********")
			}
		})
	}
}

func TestHandler_LogValuerCyclesBounded(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"direct", cycleLogValue{}}, {"group", groupCycleLogValue{}}, {"map", mapCycleLogValue{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			h := masking.NewHandler(slog.NewJSONHandler(&out, nil), masking.WithField("password", masking.FullMask()))
			slog.New(h).InfoContext(t.Context(), "test", "value", tc.value)
			require.NotEmpty(t, out.String())
			require.Less(t, out.Len(), 4096)
		})
	}
}

func TestHandler_LogValuerNestedPath(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	h := masking.NewHandler(slog.NewJSONHandler(&out, nil), masking.WithField("request.payload.password", masking.FullMask()), masking.WithMaskNestedFields())
	slog.New(h).WithGroup("request").InfoContext(t.Context(), "test", "payload", secretLogValue{})
	require.NotContains(t, out.String(), "secret-value")
	require.Contains(t, out.String(), "alice")
}

type changingLogValue struct{ calls int }

func (v *changingLogValue) LogValue() slog.Value {
	v.calls++
	if v.calls == 1 {
		return slog.StringValue("public")
	}
	return secretLogValue{}.LogValue()
}

type dualLogValue struct{}

func (dualLogValue) LogValue() slog.Value         { return secretLogValue{}.LogValue() }
func (dualLogValue) MarshalText() ([]byte, error) { return []byte("secret-value"), nil }

func TestHandler_LogValuerResolvedOnce(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	value := &changingLogValue{}
	h := masking.NewHandler(slog.NewJSONHandler(&out, nil), masking.WithField("password", masking.FullMask()))
	slog.New(h).InfoContext(t.Context(), "test", "value", value)
	require.Equal(t, 1, value.calls)
	require.Contains(t, out.String(), "public")
	require.NotContains(t, out.String(), "secret-value")
}

func TestHandler_LogValuerResolvedBeforeCustomMask(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	h := masking.NewHandler(slog.NewJSONHandler(&out, nil), masking.WithField("password", func(value string) string {
		require.Equal(t, "secret-value", value)
		return "masked"
	}))
	slog.New(h).InfoContext(t.Context(), "test", "payload", map[string]any{"password": scalarLogValue{}})
	require.Contains(t, out.String(), "masked")
}

func TestHandler_LogValuerPrecedesTextMarshaler(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	h := masking.NewHandler(slog.NewJSONHandler(&out, nil), masking.WithField("password", masking.FullMask()))
	slog.New(h).InfoContext(t.Context(), "test", "payload", struct{ Value dualLogValue }{})
	require.NotContains(t, out.String(), "secret-value")
	require.Contains(t, out.String(), "********")
}

func TestHandler_PointerInterfaceCycleBounded(t *testing.T) {
	t.Parallel()
	var value any
	value = &value
	var out bytes.Buffer
	h := masking.NewHandler(slog.NewJSONHandler(&out, nil), masking.WithField("password", masking.FullMask()))
	slog.New(h).InfoContext(t.Context(), "test", "value", value)
	require.Contains(t, out.String(), "masking: max depth exceeded")
}

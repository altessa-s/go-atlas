// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package enumcodec_test

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/converter"
	"github.com/altessa-s/go-atlas/domain/proto/enumcodec"

	"google.golang.org/genproto/googleapis/type/dayofweek"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

type grade string

const (
	gradeJunior grade = "junior"
	gradeSenior grade = "senior"
)

// protoGrade mimics a generated proto enum: GRADE_UNSPECIFIED = 0.
type protoGrade int32

const (
	gradeUnspecified protoGrade = iota
	protoJunior
	protoSenior
)

var grades = enumcodec.New(map[protoGrade]grade{protoJunior: gradeJunior, protoSenior: gradeSenior})

func TestDomain(t *testing.T) {
	t.Parallel()

	got, err := grades.Domain("attrs.grade", protoJunior)
	require.NoError(t, err)
	require.Equal(t, gradeJunior, got)

	got, err = grades.Domain("attrs.grade", gradeUnspecified)
	require.NoError(t, err, "UNSPECIFIED decodes to the zero value")
	require.Empty(t, got)

	for bad, reason := range map[protoGrade]string{3: "unknown value 3", -1: "unknown value -1"} {
		got, err = grades.Domain("attrs.grade", bad)
		require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
		fe, ok := errors.AsType[*coreerrs.FieldError](err)
		require.True(t, ok)
		require.Equal(t, "attrs.grade", fe.Field)
		require.Equal(t, reason, fe.Reason)
		require.Empty(t, got)
	}
}

func TestProto(t *testing.T) {
	t.Parallel()

	require.Equal(t, protoSenior, grades.Proto(gradeSenior))
	require.Equal(t, gradeUnspecified, grades.Proto(""), "the zero value encodes as UNSPECIFIED")
	require.Equal(t, gradeUnspecified, grades.Proto("lead"), "an unknown value encodes as UNSPECIFIED")
}

func TestRoundTripGeneratedEnum(t *testing.T) {
	t.Parallel()

	days := enumcodec.New(map[dayofweek.DayOfWeek]string{dayofweek.DayOfWeek_MONDAY: "mon", dayofweek.DayOfWeek_SUNDAY: "sun"})
	for p, d := range map[dayofweek.DayOfWeek]string{dayofweek.DayOfWeek_MONDAY: "mon", dayofweek.DayOfWeek_SUNDAY: "sun"} {
		got, err := days.Domain("day", p)
		require.NoError(t, err)
		require.Equal(t, d, got)
		require.Equal(t, p, days.Proto(d))
	}
	_, err := days.Domain("day", dayofweek.DayOfWeek_FRIDAY)
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
}

func TestNewCopiesTheMap(t *testing.T) {
	t.Parallel()

	m := map[protoGrade]grade{protoJunior: gradeJunior}
	c := enumcodec.New(m)
	m[protoSenior] = gradeSenior
	delete(m, protoJunior)

	got, err := c.Domain("g", protoJunior)
	require.NoError(t, err)
	require.Equal(t, gradeJunior, got)
	_, err = c.Domain("g", protoSenior)
	require.Error(t, err)
}

func TestNewPanicsOnBadTables(t *testing.T) {
	t.Parallel()

	for name, m := range map[string]map[protoGrade]grade{
		"zero proto value":  {gradeUnspecified: gradeJunior, protoSenior: gradeSenior},
		"zero domain value": {protoJunior: ""},
		"not one-to-one":    {protoJunior: gradeJunior, protoSenior: gradeJunior},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Panics(t, func() { enumcodec.New(m) })
		})
	}
}

func TestZeroCodec(t *testing.T) {
	t.Parallel()

	var c enumcodec.Codec[protoGrade, grade]
	got, err := c.Domain("g", gradeUnspecified)
	require.NoError(t, err)
	require.Empty(t, got)
	_, err = c.Domain("g", protoJunior)
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	require.Equal(t, gradeUnspecified, c.Proto(gradeJunior))
}

func TestConcurrentUse(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 100 {
				if d, err := grades.Domain("g", protoSenior); err != nil || d != gradeSenior {
					t.Errorf("Domain = %q, %v", d, err)
				}
				if p := grades.Proto(gradeJunior); p != protoJunior {
					t.Errorf("Proto = %d", p)
				}
			}
		})
	}
	wg.Wait()
}

func TestErrorCodec(t *testing.T) {
	t.Parallel()
	type protoItem struct {
		Grade protoGrade
		Label string
	}
	type domainItem struct {
		Grade grade
		Label string
	}
	type protoRoot struct{ Attrs protoItem }
	type domainRoot struct{ Attrs domainItem }
	for _, tc := range []struct {
		name string
		p    protoGrade
		d    grade
		bad  bool
	}{
		{"known", protoSenior, gradeSenior, false},
		{"zero", 0, "", false},
		{"unknown", 99, "unknown", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dst := domainRoot{Attrs: domainItem{Grade: gradeJunior}}
			_, err := converter.ConvertE(protoRoot{Attrs: protoItem{tc.p, "label"}}, &dst, converter.WithErrorCodecs(grades.ErrorCodec()))
			if tc.bad {
				fe, ok := err.(*coreerrs.FieldError)
				require.True(t, ok, "%T: %v", err, err)
				require.Equal(t, "attrs.grade", fe.Field)
				require.Equal(t, gradeJunior, dst.Attrs.Grade)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.d, dst.Attrs.Grade)
				require.Equal(t, "label", dst.Attrs.Label)
			}
			out := protoRoot{Attrs: protoItem{Grade: protoJunior}}
			_, err = converter.ConvertE(domainRoot{Attrs: domainItem{tc.d, "label"}}, &out, converter.WithErrorCodecs(grades.ErrorCodec()))
			if tc.bad {
				fe, ok := err.(*coreerrs.FieldError)
				require.True(t, ok)
				require.Equal(t, "attrs.grade", fe.Field)
				require.Equal(t, protoJunior, out.Attrs.Grade)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.p, out.Attrs.Grade)
			}
			p, err := grades.ProtoE(tc.d)
			if tc.bad {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.p, p)
			}
		})
	}
}

func TestErrorCodecZeroAndDelegation(t *testing.T) {
	t.Parallel()
	var c enumcodec.Codec[protoGrade, grade]
	var dst grade
	err := c.ErrorCodec()("grade", reflect.ValueOf(protoJunior), reflect.ValueOf(&dst).Elem(), nil)
	require.Error(t, err)
	called := false
	var out string
	err = c.ErrorCodec()("label", reflect.ValueOf("x"), reflect.ValueOf(&out).Elem(), func(path string, src, dst reflect.Value) error {
		called = true
		require.Equal(t, "label", path)
		dst.Set(src)
		return nil
	})
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, "x", out)
}

func TestErrorCodecFromNew(t *testing.T) {
	t.Parallel()
	adapter := enumcodec.New(map[protoGrade]grade{protoJunior: gradeJunior}).ErrorCodec()
	var dst grade
	require.NoError(t, adapter("grade", reflect.ValueOf(protoJunior), reflect.ValueOf(&dst).Elem(), nil))
	require.Equal(t, gradeJunior, dst)
}

func TestZeroErrorCodecEncode(t *testing.T) {
	t.Parallel()
	var c enumcodec.Codec[protoGrade, grade]
	var dst protoGrade
	require.Error(t, c.ErrorCodec()("grade", reflect.ValueOf(gradeJunior), reflect.ValueOf(&dst).Elem(), nil))
	require.NoError(t, c.ErrorCodec()("grade", reflect.ValueOf(grade("")), reflect.ValueOf(&dst).Elem(), nil))
	require.Equal(t, gradeUnspecified, dst)
}

func TestErrorCodecOptionalPointers(t *testing.T) {
	type protoItem struct{ Grade *protoGrade }
	type domainItem struct{ Grade *grade }
	for _, tc := range []struct {
		name string
		p    protoGrade
		d    grade
		bad  bool
	}{
		{"known", protoSenior, gradeSenior, false},
		{"zero", 0, "", false},
		{"unknown", 99, "unknown", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, d := tc.p, tc.d
			var dst struct{ Attrs domainItem }
			_, err := converter.ConvertE(struct{ Attrs protoItem }{protoItem{&p}}, &dst, converter.WithErrorCodecs(grades.ErrorCodec()))
			if tc.bad {
				var fe *coreerrs.FieldError
				if !errors.As(err, &fe) || fe.Field != "attrs.grade" {
					t.Fatalf("decode error = %v", err)
				}
			} else if err != nil || dst.Attrs.Grade == nil || *dst.Attrs.Grade != tc.d {
				t.Fatalf("decode = %+v, %v", dst, err)
			}
			var out struct{ Attrs protoItem }
			_, err = converter.ConvertE(struct{ Attrs domainItem }{domainItem{&d}}, &out, converter.WithErrorCodecs(grades.ErrorCodec()))
			if tc.bad {
				var fe *coreerrs.FieldError
				if !errors.As(err, &fe) || fe.Field != "attrs.grade" {
					t.Fatalf("encode error = %v", err)
				}
			} else if err != nil || out.Attrs.Grade == nil || *out.Attrs.Grade != tc.p {
				t.Fatalf("encode = %+v, %v", out, err)
			}
		})
	}
	t.Run("nil", func(t *testing.T) {
		p, d := protoJunior, gradeJunior
		pp, dp := &p, &d
		declined := func(string, reflect.Value, reflect.Value) error { t.Error("pointer enum delegated"); return nil }
		if err := grades.ErrorCodec()("attrs.grade", reflect.ValueOf((*protoGrade)(nil)), reflect.ValueOf(&dp).Elem(), declined); err != nil || dp != nil {
			t.Fatalf("nil decode = %v, %v", dp, err)
		}
		if err := grades.ErrorCodec()("attrs.grade", reflect.ValueOf((*grade)(nil)), reflect.ValueOf(&pp).Elem(), declined); err != nil || pp != nil {
			t.Fatalf("nil encode = %v, %v", pp, err)
		}
	})
}

func TestProtoErrorPathAndReason(t *testing.T) {
	unknown := grade("unknown")
	_, err := grades.ProtoE(unknown)
	var original *coreerrs.FieldError
	if !errors.As(err, &original) || original.Field != "" || original.Reason != "unknown value unknown" {
		t.Fatalf("ProtoE error = %#v", err)
	}
	var scalar protoGrade
	var pointer *protoGrade
	for _, tc := range []struct {
		name     string
		src, dst reflect.Value
	}{
		{"scalar", reflect.ValueOf(unknown), reflect.ValueOf(&scalar).Elem()},
		{"pointer", reflect.ValueOf(&unknown), reflect.ValueOf(&pointer).Elem()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := grades.ErrorCodec()("attrs.grade", tc.src, tc.dst, nil)
			var got *coreerrs.FieldError
			if !errors.As(err, &got) || got.Field != "attrs.grade" || got.Reason != original.Reason || got.Err != original.Err {
				t.Fatalf("adapter error = %#v, original = %#v", err, original)
			}
		})
	}
}

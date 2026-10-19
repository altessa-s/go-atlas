// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package enumcodec_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

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

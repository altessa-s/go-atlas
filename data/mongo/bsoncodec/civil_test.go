// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec_test

import (
	"bytes"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	atlascivil "github.com/altessa-s/go-atlas/core/time/civil"
	"github.com/altessa-s/go-atlas/data/mongo/bsoncodec"
)

type period struct {
	From civil.Date  `bson:"from"`
	To   *civil.Date `bson:"to"`
}

func TestCivilDateRoundTrip(t *testing.T) {
	t.Parallel()

	to := civil.Date{Year: 2026, Month: time.December, Day: 31}
	in := period{From: civil.Date{Year: 2026, Month: time.February, Day: 28}, To: &to}
	raw, err := marshal(in)
	require.NoError(t, err)
	require.Equal(t, "2026-02-28", bson.Raw(raw).Lookup("from").StringValue(), "stored as YYYY-MM-DD")
	require.Equal(t, "2026-12-31", bson.Raw(raw).Lookup("to").StringValue())

	var out period
	require.NoError(t, unmarshal(raw, &out))
	require.Equal(t, in.From, out.From)
	require.Equal(t, to, *out.To)
}

func TestCivilDateZeroAndNil(t *testing.T) {
	t.Parallel()

	raw, err := marshal(period{})
	require.NoError(t, err)
	require.Equal(t, bson.TypeNull, bson.Raw(raw).Lookup("from").Type, "the zero date is null")
	require.Equal(t, bson.TypeNull, bson.Raw(raw).Lookup("to").Type)

	var out period
	require.NoError(t, unmarshal(raw, &out))
	require.True(t, out.From.IsZero())
	require.Nil(t, out.To)
}

func TestCivilDateSortsAsStored(t *testing.T) {
	t.Parallel()

	a, err := marshal(period{From: civil.Date{Year: 999, Month: time.January, Day: 1}})
	require.NoError(t, err)
	b, err := marshal(period{From: civil.Date{Year: 2026, Month: time.January, Day: 1}})
	require.NoError(t, err)
	require.Less(t, bson.Raw(a).Lookup("from").StringValue(), bson.Raw(b).Lookup("from").StringValue(), "years are zero-padded")
}

func TestCivilDateInvalid(t *testing.T) {
	t.Parallel()

	for _, d := range []civil.Date{
		{Year: 2026, Month: time.February, Day: 30},
		{Year: 10000, Month: time.January, Day: 1},
		{Year: -1, Month: time.January, Day: 1},
	} {
		_, err := marshal(period{From: d})
		require.ErrorIs(t, err, bsoncodec.ErrInvalidDate, "%v", d)
	}

	for _, s := range []string{"2026-02-30", "2026-2-1", "26-02-01", "2026-02-01T00:00:00Z", " "} {
		raw, err := bson.Marshal(bson.M{"from": s})
		require.NoError(t, err)
		var out period
		require.ErrorIs(t, unmarshal(raw, &out), bsoncodec.ErrInvalidDate, s)
	}

	raw, err := bson.Marshal(bson.M{"from": time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	var out period
	require.Error(t, unmarshal(raw, &out), "a BSON date is an instant, not a calendar date")
}

func TestCivilDateEmptyStringDecodesAsZero(t *testing.T) {
	t.Parallel()

	raw, err := bson.Marshal(bson.M{"from": ""})
	require.NoError(t, err)
	var out period
	require.NoError(t, unmarshal(raw, &out))
	require.True(t, out.From.IsZero())
}

// serviceDate has the shape of a service's own calendar date type, with a
// method set of its own.
type serviceDate struct {
	Year  int
	Month time.Month
	Day   int
}

func (d serviceDate) IsZero() bool { return d == serviceDate{} }

type invoice struct {
	Due  serviceDate  `bson:"due"`
	Paid *serviceDate `bson:"paid"`
}

func serviceRegistry(t *testing.T, opts ...bsoncodec.DateOption) *bson.Registry {
	t.Helper()
	reg := bsoncodec.NewRegistry()
	require.NoError(t, bsoncodec.RegisterDateType[serviceDate](reg, opts...))
	return reg
}

func marshalReg(t *testing.T, reg *bson.Registry, v any) bson.Raw {
	t.Helper()
	var buf bytes.Buffer
	enc := bson.NewEncoder(bson.NewDocumentWriter(&buf))
	enc.SetRegistry(reg)
	require.NoError(t, enc.Encode(v))
	return buf.Bytes()
}

func unmarshalReg(t *testing.T, reg *bson.Registry, raw bson.Raw, v any) error {
	t.Helper()
	dec := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(raw)))
	dec.SetRegistry(reg)
	return dec.Decode(v)
}

func TestRegisterDateTypeOwnType(t *testing.T) {
	t.Parallel()

	reg := serviceRegistry(t)
	paid := serviceDate{Year: 2026, Month: time.March, Day: 1}
	raw := marshalReg(t, reg, invoice{Due: serviceDate{Year: 2026, Month: time.February, Day: 28}, Paid: &paid})
	require.Equal(t, "2026-02-28", raw.Lookup("due").StringValue())
	require.Equal(t, "2026-03-01", raw.Lookup("paid").StringValue())

	var out invoice
	require.NoError(t, unmarshalReg(t, reg, raw, &out))
	require.Equal(t, serviceDate{Year: 2026, Month: time.February, Day: 28}, out.Due)
	require.Equal(t, paid, *out.Paid)

	raw = marshalReg(t, reg, invoice{})
	require.Equal(t, bson.TypeNull, raw.Lookup("due").Type, "the zero date is null by default")
}

func TestRegisterDateTypeZeroAsEmptyString(t *testing.T) {
	t.Parallel()

	reg := serviceRegistry(t, bsoncodec.WithDateZeroAsEmptyString())
	raw := marshalReg(t, reg, invoice{})
	require.Equal(t, bson.TypeString, raw.Lookup("due").Type)
	require.Empty(t, raw.Lookup("due").StringValue(), "the zero date is the empty string")
	require.Equal(t, bson.TypeNull, raw.Lookup("paid").Type, "a nil pointer stays null")

	var out invoice
	require.NoError(t, unmarshalReg(t, reg, raw, &out))
	require.True(t, out.Due.IsZero())

	for _, bad := range []serviceDate{{Year: 2026, Month: 13, Day: 1}, {Year: 2026, Month: time.April, Day: 31}, {Year: 10000, Month: 1, Day: 1}} {
		var buf bytes.Buffer
		enc := bson.NewEncoder(bson.NewDocumentWriter(&buf))
		enc.SetRegistry(reg)
		require.ErrorIs(t, enc.Encode(invoice{Due: bad}), bsoncodec.ErrInvalidDate, "%v", bad)
	}
}

func TestRegisterDateTypeRejectsOtherTypes(t *testing.T) {
	t.Parallel()

	type noDay struct {
		Year  int
		Month time.Month
	}
	type wrongMonth struct {
		Year, Month, Day int
	}
	type unexported struct {
		Year  int
		Month time.Month
		day   int
	}
	reg := bson.NewRegistry()
	require.ErrorIs(t, bsoncodec.RegisterDateType[noDay](reg), bsoncodec.ErrNotADateType)
	require.ErrorIs(t, bsoncodec.RegisterDateType[wrongMonth](reg), bsoncodec.ErrNotADateType)
	require.ErrorIs(t, bsoncodec.RegisterDateType[unexported](reg), bsoncodec.ErrNotADateType)
	require.ErrorIs(t, bsoncodec.RegisterDateType[string](reg), bsoncodec.ErrNotADateType)
	_ = unexported{}.day
}

type atlasPeriod struct {
	From atlascivil.Date  `bson:"from"`
	End  atlascivil.Date  `bson:"end"`
	To   *atlascivil.Date `bson:"to"`
}

func TestAtlasCivilDateRegisteredByDefault(t *testing.T) {
	t.Parallel()

	in := atlasPeriod{From: atlascivil.MustNew(2026, time.February, 28)}
	raw, err := marshal(in)
	require.NoError(t, err)
	require.Equal(t, "2026-02-28", bson.Raw(raw).Lookup("from").StringValue())
	require.Equal(t, bson.TypeString, bson.Raw(raw).Lookup("end").Type, "the zero date is stored as its text form")
	require.Empty(t, bson.Raw(raw).Lookup("end").StringValue())
	require.Equal(t, bson.TypeNull, bson.Raw(raw).Lookup("to").Type)

	var out atlasPeriod
	require.NoError(t, unmarshal(raw, &out))
	require.Equal(t, in, out)
}

// TestDateTypeValidRule pins that a date type's own Valid() rule bounds the
// codec: go-atlas civil.Date excludes year 0, Google's civil.Date does not.
func TestDateTypeValidRule(t *testing.T) {
	t.Parallel()

	_, err := marshal(atlasPeriod{From: atlascivil.Date{Year: 0, Month: time.January, Day: 1}})
	require.ErrorIs(t, err, bsoncodec.ErrInvalidDate, "year 0 is not a go-atlas civil date")

	raw, err := bson.Marshal(bson.M{"from": "0000-01-01"})
	require.NoError(t, err)
	var out atlasPeriod
	require.ErrorIs(t, unmarshal(raw, &out), bsoncodec.ErrInvalidDate)

	var google period
	require.NoError(t, unmarshal(raw, &google), "Google's civil.Date accepts year 0")
	require.Equal(t, 0, google.From.Year)
}

// pointerValidDate has its validity rule on the pointer receiver.
type pointerValidDate struct {
	Year  int
	Month time.Month
	Day   int
}

func (d *pointerValidDate) Valid() bool { return d.Year >= 1 }

func TestDateTypeValidRulePointerReceiver(t *testing.T) {
	t.Parallel()

	reg := bsoncodec.NewRegistry()
	require.NoError(t, bsoncodec.RegisterDateType[pointerValidDate](reg))
	type doc struct {
		D pointerValidDate `bson:"d"`
	}
	var buf bytes.Buffer
	enc := bson.NewEncoder(bson.NewDocumentWriter(&buf))
	enc.SetRegistry(reg)
	require.ErrorIs(t, enc.Encode(doc{D: pointerValidDate{Year: 0, Month: time.January, Day: 1}}), bsoncodec.ErrInvalidDate)

	raw, err := bson.Marshal(bson.M{"d": "0000-01-01"})
	require.NoError(t, err)
	var out doc
	require.ErrorIs(t, unmarshalReg(t, reg, raw, &out), bsoncodec.ErrInvalidDate)

	ok, err := bson.Marshal(bson.M{"d": "2026-10-09"})
	require.NoError(t, err)
	require.NoError(t, unmarshalReg(t, reg, ok, &out))
	require.Equal(t, 2026, out.D.Year)
}

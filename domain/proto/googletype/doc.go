// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package googletype converts between the google.type and
// google.protobuf well-known types an API carries and the Go values a
// domain uses.
//
// [MoneyFromProto] and [MoneyToProto] map google.type.Money to money.Money
// exactly, [DateFromProto] and [DateToProto] map google.type.Date to
// civil.Date (absent = the zero date; an impossible or partial date is
// rejected), [WeekdayFromProto], [WeekdaysFromProto], [WeekdayToProto] and
// [WeekdaysToProto] map google.type.DayOfWeek to time.Weekday, and
// [DurationFromProto], [DurationToProto] and [TimestampToProto] treat a zero
// value as absent.
//
// # Errors
//
// The converters do conversion only. A decoder rejects input the wire type
// cannot represent with a sentinel error (classify with errors.Is) that
// carries no API field path; a list decoder wraps the element's error in an
// [*IndexError]. Domain policy — a non-negative amount, an expected
// currency — and the mapping onto request field paths belong to the caller:
//
//	days, err := googletype.WeekdaysFromProto(req.GetBot().GetDigestSchedule().GetDays())
//	if ie, ok := errors.AsType[*googletype.IndexError](err); ok {
//		return nil, coreerrs.NewFieldError(fmt.Sprintf("bot.digestSchedule.days[%d]", ie.Index), "unknown day of the week")
//	}
package googletype

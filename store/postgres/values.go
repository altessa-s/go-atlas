// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Missing maps absent rows to the domain error supplied by the storage.
func Missing(err, missing error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return missing
	}
	return err
}

// Changed classifies a guarded write that matched no rows.
func Changed(rows int64, err, missing error) error {
	if err == nil && rows == 0 {
		return missing
	}
	return err
}

// Interval encodes durations at PostgreSQL's microsecond resolution.
func Interval(d time.Duration) (pgtype.Interval, error) {
	if d%time.Microsecond != 0 {
		return pgtype.Interval{}, errors.New("duration must be a whole number of microseconds")
	}
	return pgtype.Interval{Microseconds: d.Microseconds(), Valid: true}, nil
}

// Duration decodes a nonnegative elapsed time interval. Calendar months have no fixed duration.
func Duration(i pgtype.Interval) (time.Duration, error) {
	if !i.Valid || i.Months != 0 {
		return 0, errors.New("invalid elapsed-time interval")
	}
	// PostgreSQL stores days and microseconds independently. Combine their signs
	// before testing the total, without multiplying an unbounded day count.
	const microsPerDay = int64(24 * time.Hour / time.Microsecond)
	const maxMicros = math.MaxInt64 / int64(time.Microsecond)
	days := int64(i.Days) + i.Microseconds/microsPerDay
	micros := i.Microseconds % microsPerDay
	if micros < 0 {
		days--
		micros += microsPerDay
	}
	if days < 0 || days > (maxMicros-micros)/microsPerDay {
		return 0, errors.New("interval exceeds duration range")
	}
	return time.Duration(days*microsPerDay+micros) * time.Microsecond, nil
}

// BatchSize bounds the row count passed to a worker query.
func BatchSize(n int) (int32, error) {
	if n <= 0 || int64(n) > math.MaxInt32 {
		return 0, errors.New("batch size must be a positive 32-bit integer")
	}
	return int32(n), nil
}

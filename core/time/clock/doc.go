// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package clock provides an injectable source of the current time that can also wait, and a manually driven fake for tests.
//
// [Clock] has two methods, Now and Sleep(ctx, d). [Real] is the system clock; [Fake] returns a set time, moves it with Advance
// and Set, and makes Sleep advance the time at once instead of waiting, so code with timeouts, TTLs or backoff runs
// deterministically and fast in tests. The method values fit the existing seams: Now as a func() time.Time, Sleep as a
// retry.SleepFunc.
//
//	type Service struct{ clock clock.Clock }
//
//	// production
//	svc := Service{clock: clock.Real()}
//
//	// test
//	fake := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
//	svc := Service{clock: fake}
//	fake.Advance(time.Hour)
//	err := retry.Do(ctx, op, retry.WithSleep(fake.Sleep))
package clock

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package errstatustest runs gRPC handler calls in unit tests through the
// same errstatus server interceptor the server installs, so a test sees the
// error a client would: a domain error converted by the configured rules, a
// status the handler returned passed through, anything else
// codes.Internal.
//
// # Usage
//
// Build one [Harness] per test package with the server's errstatus options
// and call the handler through it:
//
//	var harness = errstatustest.New(
//		errstatus.WithCanonicalAPIRules(),
//		errstatus.WithFinalizer(errstatus.DefaultFinalizerWithDomain("svc")),
//	)
//
//	got, err := errstatustest.Invoke(t, harness, h, botv1.BotService_GetBot_FullMethodName, h.GetBot, req)
//	err = harness.Convert(t, h.check(req)) // the error of a helper called directly
package errstatustest

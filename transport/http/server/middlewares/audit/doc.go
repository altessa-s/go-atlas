// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package audit provides HTTP middleware that emits audit events for every
// processed request. It captures method, path, status code, duration, and
// caller identity via the [audit.Auditor] pipeline.
package audit

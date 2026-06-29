// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package dataaudit bridges the authorization-decision [audit.Sink] seam to the
// project's general-purpose data/audit dispatcher. It is the wiring that keeps
// the auth/audit primitive free of any dependency on data/audit: the core
// declares the Sink seam, this adapter satisfies it.
//
// A [Sink] maps each [audit.Decision] to a data/audit Event of type
// EventTypeAuth — the subject becomes the actor, the action key and optional
// resource become the event resource, the outcome maps to a success or denied
// result, and RequiredScope plus the decision attributes ride along in the event
// metadata — then emits it through a started *audit.Auditor (data/audit). A
// dropped event (full buffer) surfaces as [ErrDropped] so an audit.Recorder
// under FailureRequired can fail the request.
//
// # Usage
//
//	auditor, _ := dataudit.New(dispatcher)   // data/audit, dispatcher already started
//	_ = auditor.Start()
//	rec := audit.NewRecorder(dataaudit.New(auditor))
package dataaudit

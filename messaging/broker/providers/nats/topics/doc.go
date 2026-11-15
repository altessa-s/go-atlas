// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package topics provides type-safe NATS subject templating with macro
// placeholders.
//
// A [Topic] is a string template such as "events.{TENANT}" containing
// curly-brace macros. Substitute macros at runtime to produce a concrete
// subject ready for NATS publish or subscribe operations.
//
// [Topic.With] panics on invalid input and is intended for trusted, in-process
// call sites. [Topic.WithValidation] returns an error and additionally enforces
// [MaxSubjectLength] and wildcard positioning - prefer it whenever any input is
// untrusted.
//
// # Example
//
//	const tenantEvents topics.Topic = "events.{TENANT}"
//
//	subject := tenantEvents.With("TENANT", "acme")
//	// subject == "events.acme"
//
//	subject, err := tenantEvents.WithValidation("TENANT", externalInput)
//	if err != nil {
//	    return err
//	}
//
// All operations on [Topic] and [TopicKey] are safe for concurrent use.
// Parsed macros are cached in a process-global map keyed by topic template,
// so the cache is bounded by the number of distinct templates declared.
package topics

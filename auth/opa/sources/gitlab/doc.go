// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package gitlab provides a [opa.PolicySource] implementation that reads OPA policies
// from a GitLab repository.
//
// The source uses the GitLab API to list and download .rego policy files (and optionally
// .json data files) from a configurable project path and ref. Change detection is handled
// by the Manager via periodic polling.
//
// Example usage:
//
//	source, err := gitlab.New(
//	    gitlab.WithEndpoint("https://gitlab.example.com"),
//	    gitlab.WithToken(token),
//	    gitlab.WithProjectID(42),
//	    gitlab.WithRef("main"),
//	    gitlab.WithDir("policies/opa"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer source.Close()
//
//	bundle, err := source.Fetch(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
package gitlab

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package embed implements [opa.PolicySource] backed by an [fs.FS]
// (typically an [embed.FS]) containing .rego policy files.
//
// This allows OPA policies to be compiled directly into the binary,
// eliminating the need for external policy files at runtime.
// Change detection is handled by the [opa.Manager].
//
// # Usage
//
//	//go:embed policies/*
//	var policiesFS embed.FS
//
//	source, err := embed.New(policiesFS, "policies")
//	if err != nil {
//	    return err
//	}
//	defer source.Close()
//
//	bundle, err := source.Fetch(ctx)
package embed

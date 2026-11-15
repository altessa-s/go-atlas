// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redacted holds interoperability tests of core/types/redacted with
// third-party encoders (gopkg.in/yaml.v3 and the MongoDB BSON codec). They
// live outside core so the core module does not require those libraries,
// even for tests.
package redacted

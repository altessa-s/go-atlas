// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl

// ParsePolicy converts a string policy name to a Policy value.
// Unrecognized values default to PolicyDeny (fail-closed).
func ParsePolicy(s string) Policy {
	switch s {
	case "allow":
		return PolicyAllow
	case "deny":
		return PolicyDeny
	default:
		return PolicyDeny
	}
}

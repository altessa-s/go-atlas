// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package transportconfig

import (
	"errors"
	"fmt"
	"regexp"
)

// ErrInvalidPublicMethod indicates a malformed public gRPC cache method name.
var ErrInvalidPublicMethod = errors.New("public method must be a full gRPC method name such as /service.Service/Method")

var fullRPCMethodName = regexp.MustCompile(`^/[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*/[A-Za-z_][A-Za-z0-9_]*$`)

// ValidatePublicMethod checks the syntax of a public gRPC cache method name.
func ValidatePublicMethod(method string) error {
	if !fullRPCMethodName.MatchString(method) {
		return fmt.Errorf("%w: %q", ErrInvalidPublicMethod, method)
	}
	return nil
}

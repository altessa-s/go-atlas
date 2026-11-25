// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"errors"
	"fmt"

	"github.com/altessa-s/go-atlas/internal/diagnostics"

	httpclient "github.com/altessa-s/go-atlas-net/http/client"
)

// responseFailure carries the numeric status through a storage or rebuild
// wrapper. Its embedded error contains only the fixed diagnostic.
type responseFailure struct {
	error
	status int
}

func (e *responseFailure) Is(target error) bool { return errors.Is(e.error, target) }

// boundaryError keeps arbitrary endpoint and storage failures available to
// errors.Is without rendering them or exposing them through errors.Unwrap.
func boundaryError(op string, status int, kind, cause error) error {
	if status == 0 {
		if response, ok := errors.AsType[*responseFailure](cause); ok {
			status = response.status
		} else if upstream := httpclient.IsUnexpectedStatusError(cause); upstream != nil {
			// The factory transport can return an HTTP failure through Do's
			// error result. Its URL and response preview stay private.
			status = upstream.Status
		}
	}
	message := fmt.Sprintf("%s: status=%d cause=%s", op, status, diagnostics.ErrorClass(cause))
	return &responseFailure{error: diagnostics.ClassifiedError(message, errors.Join(kind, cause)), status: status}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metadata

import (
	"context"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
)

// metadataInterceptor handles initialization of CallMetadata in the context.
type metadataInterceptor struct{}

func (i metadataInterceptor) Name() string { return "metadata" }

// Dependencies returns nil as metadata has no dependencies.
// Metadata is the root interceptor that all others depend on.
func (i metadataInterceptor) Dependencies() []string { return nil }

func (i metadataInterceptor) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	// Metadata is already passed in 'c' and injected into context by the caller
	// of DrivenInterceptor if we use our standard wrappers.
	return driver.NoopDriver(), ctx
}

// Interceptor returns a [driver.DrivenInterceptor] that ensures [CallMetadata]
// is present in the context for all downstream interceptors. The [Chain]
// automatically prepends this interceptor if none is registered with the
// name "metadata".
func Interceptor() driver.DrivenInterceptor {
	return metadataInterceptor{}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"

	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
)

// options holds [Factory] configuration. The logger defaults to a discard
// handler; cacheMetadataProcessor and tlsProviders are optional.
type options struct {
	logger                 *slog.Logger
	cacheMetadataProcessor cache.MetadataProcessor `optgen:"notnil"`
	tlsProviders           *tlsproviders.Providers
}

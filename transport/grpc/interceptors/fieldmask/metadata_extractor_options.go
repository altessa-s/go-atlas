// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

//go:generate go tool optgen generate --type=metadataExtractorOptions --option-type=MetadataExtractorOption --output=metadata_extractor_options_gen.go

// metadataExtractorOptions configures [MetadataReadExtractor] through the
// generated [MetadataExtractorOption] values.
type metadataExtractorOptions struct {
	// metadataHeader is the gRPC metadata key the extractor reads. An empty
	// name is ignored and the default is kept.
	metadataHeader string `optgen:"default=DefaultMetadataReadMaskHeader"`
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/MicahParks/jwkset"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coreio "github.com/altessa-s/go-atlas-core/io"
)

// fetchJWKS bounds the complete document before any key conversion or publication.
func (p *jwksKeySource) fetchJWKS(ctx context.Context) (jwkset.JWKSMarshal, error) {
	var jwks jwkset.JWKSMarshal
	if p.info == nil || p.info.JwksURL == "" {
		return jwks, coreerrs.Wrap(ErrDiscovery, "JWKS URL not available")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.info.JwksURL, nil)
	if err != nil {
		return jwks, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return jwks, err
	}
	// Do not drain a hostile oversized stream after reaching the read limit.
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return jwks, coreerrs.Wrapf(errors.Join(ErrDiscovery, jwkset.ErrInvalidHTTPStatusCode), "unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(coreio.NewLimitedReadCloser(resp.Body, maxJWKSResponseSize))
	if errors.Is(err, coreio.ErrReadLimitExceeded) {
		return jwks, ErrJWKSTooLarge
	}
	if err != nil {
		return jwks, coreerrs.WrapOperation(err, "read JWK Set")
	}
	// JWKMarshal maps both an omitted and a null key_ops to a nil slice.
	// Preserve presence so only omission permits unrestricted operations.
	var doc struct {
		Keys []struct {
			jwkset.JWKMarshal
			Operations json.RawMessage `json:"key_ops"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return jwks, coreerrs.WrapOperation(err, "decode JWK Set")
	}
	if len(doc.Keys) > maxJWKSKeys {
		return jwks, ErrJWKSTooLarge
	}
	for _, key := range doc.Keys {
		if key.Operations != nil {
			if err := json.Unmarshal(key.Operations, &key.KEYOPS); err != nil {
				return jwks, coreerrs.WrapOperation(err, "decode JWK key operations")
			}
			if key.KEYOPS == nil {
				key.KEYOPS = []jwkset.KEYOPS{}
			}
		}
		jwks.Keys = append(jwks.Keys, key.JWKMarshal)
	}
	return jwks, nil
}

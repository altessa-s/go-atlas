// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package token

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/altessa-s/go-atlas/security/vault/auth"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
	vaultApi "github.com/hashicorp/vault/api"
)

// AuthMethod implements direct token authentication for Vault.
// It validates and uses an existing Vault token.
// The token is stored in a SecureString and zeroed on Shutdown.
type AuthMethod struct {
	token *corestrings.SecureString
}

// New creates a new token authentication method.
// The token must be a valid Vault token (e.g., "hvs.CAESIJ...").
//
// Example:
//
//	authMethod := token.New("hvs.CAESIJ...")
func New(token string) *AuthMethod {
	return &AuthMethod{token: corestrings.NewSecureString(strings.TrimSpace(token))}
}

// Authenticate validates the token and returns its metadata.
// The token is set on the client and a lookup is performed to verify validity.
func (a *AuthMethod) Authenticate(ctx context.Context, client *vaultApi.Client) (*vaultApi.Secret, error) {
	client.SetToken(a.token.String())

	secret, err := client.Auth().Token().LookupSelfWithContext(ctx)
	if err != nil {
		return nil, auth.WrapAuthError("token", err)
	}

	if secret == nil {
		return nil, auth.WrapAuthError("token", auth.ErrEmptyResponse)
	}

	// For some reason, the token lookup returns token data in the `Data` field
	// but the renewals of tokens return it in the proper `Auth` field.  This
	// seems like a Vault bug or at least a major inconsistency.
	if secret.Auth == nil {
		secret.Auth = &vaultApi.SecretAuth{
			ClientToken: a.token.String(),
		}

		var ok bool

		if secret.Auth.Renewable, ok = secret.Data["renewable"].(bool); ok {
			if ttl, ok := secret.Data["ttl"].(json.Number); ok {
				if ttlInt, err := ttl.Int64(); err == nil {
					secret.Auth.LeaseDuration = int(ttlInt)
				}
			}
		}
	}

	return secret, nil
}

// Name returns the authentication method name.
func (a *AuthMethod) Name() string { return "token" }

// Shutdown zeros the token in memory and releases resources.
func (a *AuthMethod) Shutdown() error {
	if a.token != nil {
		a.token.Clear()
		a.token = nil
	}
	return nil
}

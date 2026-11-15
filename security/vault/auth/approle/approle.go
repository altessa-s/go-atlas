// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package approle

import (
	"strings"

	"github.com/hashicorp/vault/api/auth/approle"

	"github.com/altessa-s/go-atlas-core/text/strings/secure"
	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

func newLogin(roleID string, secretID *secure.String) func(opts ...approle.LoginOption) (vaultApi.AuthMethod, error) {
	return func(opts ...approle.LoginOption) (vaultApi.AuthMethod, error) {
		return approle.NewAppRoleAuth(roleID, &approle.SecretID{FromString: secretID.String()}, opts...)
	}
}

// Option configures the AppRole authentication method.
type Option = auth.MethodOption[*auth.MountPathMethod]

// New creates a new AppRole authentication method. The secret ID is held in a
// SecureString and zeroed when the method's Shutdown runs, so it does not remain
// in the heap for the process lifetime.
func New(roleID, secretID string, opt ...Option) *auth.MountPathMethod {
	roleID = strings.TrimSpace(roleID)
	sid := secure.NewString(strings.TrimSpace(secretID))

	opt = append(opt, auth.WithSecret(sid))
	return auth.NewMountPathMethodOptions("approle", approle.WithMountPath, newLogin(roleID, sid), opt...)
}

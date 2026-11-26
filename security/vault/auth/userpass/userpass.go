// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package userpass

import (
	"strings"

	"github.com/hashicorp/vault/api/auth/userpass"

	"github.com/altessa-s/go-atlas-core/text/strings/secure"
	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

// Option configures the userpass authentication method.
type Option = auth.MethodOption[*auth.MountPathMethod]

// New creates a new userpass authentication method. The password is held in a
// SecureString and zeroed when the method's Shutdown runs, so it does not remain
// in the heap for the process lifetime.
func New(username, password string, opt ...Option) *auth.MountPathMethod {
	username = strings.TrimSpace(username)
	pw := secure.NewString(password)

	opt = append(opt, auth.WithSecret(pw))
	return auth.NewMountPathMethodOptions("userpass", userpass.WithMountPath, func(opts ...userpass.LoginOption) (vaultApi.AuthMethod, error) {
		return userpass.NewUserpassAuth(username, &userpass.Password{FromString: pw.String()}, opts...)
	}, opt...)
}

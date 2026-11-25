// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package architecture_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/auth/selfjwt"
)

// A caller must only be able to pass options intended for the constructor's role.
func TestAuthConstructorOptionsAreRoleSpecific(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		constructors []any
	}{
		{"jwt", []any{jwt.NewSigner, jwt.NewVerifier}},
		{"selfjwt", []any{selfjwt.NewMinter, selfjwt.NewVerifier, selfjwt.New}},
		{"oauth2client", []any{oauth2client.ClientCredentials, oauth2client.Refresh, oauth2client.NewAuthCode,
			oauth2client.NewDeviceFlow, oauth2client.NewExchanger, oauth2client.NewJWTBearer, oauth2client.NewRevoker}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var seen []reflect.Type
			for _, constructor := range tc.constructors {
				fn := reflect.TypeOf(constructor)
				require.True(t, fn.IsVariadic())
				option := fn.In(fn.NumIn() - 1).Elem()
				for _, other := range seen {
					require.False(t, option.AssignableTo(other), "%v is accepted as %v", option, other)
					require.False(t, other.AssignableTo(option), "%v is accepted as %v", other, option)
				}
				seen = append(seen, option)
			}
		})
	}
}

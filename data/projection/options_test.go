// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package projection_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/projection"
)

func TestNewTranslatorContextErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    []projection.TranslatorOption
		wantErr error
	}{
		{
			name:    "untrusted without allow-list",
			opts:    []projection.TranslatorOption{projection.WithUntrustedInput()},
			wantErr: projection.ErrAllowlistRequired,
		},
		{
			name:    "untrusted with empty allow-list",
			opts:    []projection.TranslatorOption{projection.WithUntrustedInput(), projection.WithAllowedFields()},
			wantErr: projection.ErrAllowlistRequired,
		},
		{
			name:    "empty allow-list has no default",
			opts:    []projection.TranslatorOption{projection.WithAllowedFields()},
			wantErr: projection.ErrDefaultFieldsRequired,
		},
		{
			name:    "malformed denied path",
			opts:    []projection.TranslatorOption{projection.WithDeniedFields("a..b")},
			wantErr: projection.ErrInvalidFieldPath,
		},
		{
			name:    "unsupported required path",
			opts:    []projection.TranslatorOption{projection.WithRequiredFields("items.0")},
			wantErr: projection.ErrUnsupportedPath,
		},
		{
			name: "required overlaps denied storage",
			opts: []projection.TranslatorOption{
				projection.WithRequiredFields("secret.hash"),
				projection.WithDeniedStorageFields("secret"),
			},
			wantErr: projection.ErrConflictingPolicy,
		},
		{
			name: "required overlaps mapped denied field",
			opts: []projection.TranslatorOption{
				projection.WithFieldMapping(map[string]string{"password": "pwd"}),
				projection.WithDeniedFields("password"),
				projection.WithRequiredFields("pwd"),
			},
			wantErr: projection.ErrConflictingPolicy,
		},
		{
			name: "default field outside allow-list",
			opts: []projection.TranslatorOption{
				projection.WithAllowedFields("name"),
				projection.WithDefaultFields("email"),
			},
			wantErr: projection.ErrConflictingPolicy,
		},
		{
			name: "default field denied",
			opts: []projection.TranslatorOption{
				projection.WithDefaultFields("credentials"),
				projection.WithDeniedFields("credentials.password"),
			},
			wantErr: projection.ErrConflictingPolicy,
		},
		{
			name: "allow-list root carries a denied field",
			opts: []projection.TranslatorOption{
				projection.WithAllowedFields("name", "credentials.*"),
				projection.WithDeniedFields("credentials.password"),
			},
			wantErr: projection.ErrDefaultFieldsRequired,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := projection.NewTranslatorContext(tc.opts...)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()

	ctx, err := projection.NewTranslatorContext(
		projection.WithUntrustedInput(),
		projection.WithAllowedFields("name", "email", "address.*", "credentials.*", "createTime", "alias"),
		projection.WithDeniedFields("credentials.password"),
		projection.WithDeniedStorageFields("secret_col"),
		projection.WithFieldMapping(map[string]string{"createTime": "created_at", "alias": "secret_col"}),
		projection.WithFieldPrefixMapping(map[string]string{"address.": "addr."}),
		projection.WithRequiredFields("_id"),
		projection.WithDefaultFields("name", "email"),
	)
	require.NoError(t, err)

	tests := []struct {
		name    string
		paths   []string
		want    []string
		wantErr error
	}{
		{name: "empty uses default", want: []string{"_id", "email", "name"}},
		{name: "mapped and required", paths: []string{"createTime", "name"}, want: []string{"_id", "created_at", "name"}},
		{name: "prefix mapping", paths: []string{"address.city"}, want: []string{"_id", "addr.city"}},
		{name: "allowed sibling of denied", paths: []string{"credentials.login"}, want: []string{"_id", "credentials.login"}},
		{name: "not allowed", paths: []string{"phone"}, wantErr: projection.ErrFieldNotAllowed},
		{name: "bare parent of wildcard", paths: []string{"address"}, wantErr: projection.ErrFieldNotAllowed},
		{name: "denied exact", paths: []string{"credentials.password"}, wantErr: projection.ErrFieldNotAllowed},
		{name: "denied descendant", paths: []string{"credentials.password.hash"}, wantErr: projection.ErrFieldNotAllowed},
		{name: "alias onto denied storage", paths: []string{"alias"}, wantErr: projection.ErrFieldNotAllowed},
		{name: "hand-built operator path", paths: []string{"$where"}, wantErr: projection.ErrInvalidFieldPath},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sel, err := ctx.Resolve(projection.Spec{Paths: tc.paths})
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, sel.Include)
			require.Empty(t, sel.Exclude)
		})
	}
}

func TestResolveDeniedAncestor(t *testing.T) {
	t.Parallel()

	ctx, err := projection.NewTranslatorContext(projection.WithDeniedFields("credentials.password"))
	require.NoError(t, err)

	_, err = ctx.Resolve(projection.Spec{Paths: []string{"credentials"}})
	require.ErrorIs(t, err, projection.ErrFieldNotAllowed, "a parent would carry the denied child along")
}

func TestDefaultSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		opts        []projection.TranslatorOption
		wantInclude []string
		wantExclude []string
	}{
		{name: "no policy selects everything"},
		{name: "lone star selects everything", opts: []projection.TranslatorOption{projection.WithAllowedFields("*")}},
		{
			name:        "denied fields become an exclusion",
			opts:        []projection.TranslatorOption{projection.WithDeniedFields("password"), projection.WithDeniedStorageFields("token")},
			wantExclude: []string{"password", "token"},
		},
		{
			name:        "allow-list roots",
			opts:        []projection.TranslatorOption{projection.WithAllowedFields("name", "address.*", "address.city")},
			wantInclude: []string{"address", "address.city", "name"},
		},
		{
			name: "allow-list roots are mapped and joined by required fields",
			opts: []projection.TranslatorOption{
				projection.WithAllowedFields("createTime"),
				projection.WithFieldMapping(map[string]string{"createTime": "created_at"}),
				projection.WithRequiredFields("id"),
			},
			wantInclude: []string{"created_at", "id"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, err := projection.NewTranslatorContext(tc.opts...)
			require.NoError(t, err)

			def := ctx.DefaultSelection()
			require.Equal(t, tc.wantInclude, def.Include)
			require.Equal(t, tc.wantExclude, def.Exclude)
			require.Equal(t, len(tc.wantInclude)+len(tc.wantExclude) == 0, def.IsAll())

			if len(def.Include) > 0 {
				def.Include[0] = "mutated"
				require.NotEqual(t, "mutated", ctx.DefaultSelection().Include[0], "DefaultSelection returns a copy")
			}
		})
	}
}

func TestMappingCannotBypassDenial(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts []projection.TranslatorOption
		path string
	}{
		{
			name: "alias onto a remapped descendant of a denied field",
			opts: []projection.TranslatorOption{
				projection.WithDeniedFields("credentials"),
				projection.WithFieldMapping(map[string]string{"credentials.password": "pwd", "alias": "pwd"}),
			},
			path: "alias",
		},
		{
			name: "alias under a denied field's prefix target",
			opts: []projection.TranslatorOption{
				projection.WithDeniedFields("credentials"),
				projection.WithFieldPrefixMapping(map[string]string{"credentials.": "cred."}),
				projection.WithFieldMapping(map[string]string{"alias": "cred.password"}),
			},
			path: "alias",
		},
		{
			name: "alias under an exact ancestor's target",
			opts: []projection.TranslatorOption{
				projection.WithDeniedFields("credentials.password"),
				projection.WithFieldMapping(map[string]string{"credentials": "c", "alias": "c.password"}),
			},
			path: "alias",
		},
		{
			name: "alias under a prefix target shadowed by an exact entry",
			opts: []projection.TranslatorOption{
				projection.WithDeniedFields("credentials.password"),
				projection.WithFieldPrefixMapping(map[string]string{"credentials.": "c."}),
				projection.WithFieldMapping(map[string]string{"credentials.password": "secret", "alias": "c"}),
			},
			path: "alias",
		},
		{
			name: "parent whose moved descendant is denied in storage",
			opts: []projection.TranslatorOption{
				projection.WithDeniedStorageFields("pwd"),
				projection.WithFieldMapping(map[string]string{"credentials.password": "pwd"}),
			},
			path: "credentials",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, err := projection.NewTranslatorContext(tc.opts...)
			require.NoError(t, err)
			_, err = ctx.Resolve(projection.Spec{Paths: []string{tc.path}})
			require.ErrorIs(t, err, projection.ErrFieldNotAllowed)
		})
	}
}

func TestDeniedRemappedDescendantExcludedByDefault(t *testing.T) {
	t.Parallel()

	ctx, err := projection.NewTranslatorContext(
		projection.WithDeniedFields("credentials"),
		projection.WithFieldMapping(map[string]string{"credentials.password": "pwd"}),
	)
	require.NoError(t, err)
	require.Equal(t, []string{"credentials", "pwd"}, ctx.DefaultSelection().Exclude)
}

func TestSubtreeSelection(t *testing.T) {
	t.Parallel()

	ctx, err := projection.NewTranslatorContext(
		projection.WithAllowedFields("name", "address.*"),
		projection.WithFieldPrefixMapping(map[string]string{"address.": "addr."}),
		projection.WithFieldMapping(map[string]string{"address.city": "city_col"}),
	)
	require.NoError(t, err)

	require.Equal(t, []string{"addr", "city_col", "name"}, ctx.DefaultSelection().Include,
		"a wildcard root follows its prefix rule and keeps moved descendants")

	sel, err := ctx.Resolve(projection.Spec{Paths: []string{"address.zip", "address.city"}})
	require.NoError(t, err)
	require.Equal(t, []string{"addr.zip", "city_col"}, sel.Include)
}

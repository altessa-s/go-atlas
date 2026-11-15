// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	mongoconfig "github.com/altessa-s/go-atlas/config/mongo"
)

// TestMongoKMS_LoadsFromYAML pins the KMS provider selector values and
// section keys to the backend names under store/mongo/kms/providers.
func TestMongoKMS_LoadsFromYAML(t *testing.T) {
	t.Parallel()

	cfg := loadYAML[mongoconfig.KMS](t, "provider: aws\naws:\n  accessKeyID: id\n  secretAccessKey: secret\n  key: arn\n")
	require.Equal(t, mongoconfig.KMSProviderAWS, cfg.Provider)
	require.NotNil(t, cfg.AWS)
	require.Equal(t, "id", cfg.AWS.AccessKeyId)
	require.NoError(t, cfg.Validate())

	cfg = loadYAML[mongoconfig.KMS](t, "provider: gcp\ngcp:\n  projectID: p\n  email: e\n  privateKey: k\n  location: l\n  keyRing: r\n  keyName: n\n")
	require.Equal(t, mongoconfig.KMSProviderGCP, cfg.Provider)
	require.NotNil(t, cfg.GCP)
	require.Equal(t, "p", cfg.GCP.ProjectId)
	require.NoError(t, cfg.Validate())
}

func TestMongoKMS_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kms     mongoconfig.KMS
		wantErr bool
	}{
		{name: "aws without section", kms: mongoconfig.KMS{Provider: mongoconfig.KMSProviderAWS}, wantErr: true},
		{name: "gcp without section", kms: mongoconfig.KMS{Provider: mongoconfig.KMSProviderGCP}, wantErr: true},
		{name: "legacy amazon value", kms: mongoconfig.KMS{Provider: "amazon"}, wantErr: true},
		{name: "legacy google value", kms: mongoconfig.KMS{Provider: "google"}, wantErr: true},
		{
			name: "aws with section",
			kms: mongoconfig.KMS{
				Provider: mongoconfig.KMSProviderAWS,
				AWS:      &mongoconfig.KMSAWS{AccessKeyId: "id", SecretAccessKey: "secret", Key: "arn"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.kms.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

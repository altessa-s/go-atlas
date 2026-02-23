// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package convert

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuoteYAMLSpecialValues(t *testing.T) {
	conv := NewConfigToEnvConverter()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "asterisk value",
			input:    `  maskString: ****`,
			expected: `  maskString: "****"`,
		},
		{
			name:     "ampersand anchor",
			input:    `  anchor: &name value`,
			expected: `  anchor: "&name value"`,
		},
		{
			name:     "exclamation tag",
			input:    `  typed: !custom data`,
			expected: `  typed: "!custom data"`,
		},
		{
			name:     "at sign reserved",
			input:    `  reserved: @value`,
			expected: `  reserved: "@value"`,
		},
		{
			name:     "backtick reserved",
			input:    "  code: `value`",
			expected: "  code: \"`value`\"",
		},
		{
			name:     "normal value unchanged",
			input:    `  key: normal-value`,
			expected: `  key: normal-value`,
		},
		{
			name:     "already double quoted",
			input:    `  key: "****"`,
			expected: `  key: "****"`,
		},
		{
			name:     "already single quoted",
			input:    `  key: '****'`,
			expected: `  key: '****'`,
		},
		{
			name:     "empty value unchanged",
			input:    `  key:`,
			expected: `  key:`,
		},
		{
			name:     "no colon unchanged",
			input:    `  - list item`,
			expected: `  - list item`,
		},
		{
			name:     "value with embedded quotes",
			input:    `  key: *"quoted"`,
			expected: `  key: "*\"quoted\""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := conv.quoteYAMLSpecialValues(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessLineQuotesSpecialValues(t *testing.T) {
	conv := NewConfigToEnvConverter()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "commented maskString with asterisks",
			input:    `#  maskString: ****`,
			expected: `  maskString: "****"`,
		},
		{
			name:     "commented normal value",
			input:    `#  level: error`,
			expected: `  level: error`,
		},
		{
			name:     "non-YAML comment unchanged",
			input:    `# This is a description`,
			expected: `# This is a description`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := conv.processLine(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUncommentYAMLWithSpecialValues(t *testing.T) {
	conv := NewConfigToEnvConverter()

	input := `# Logger config
#logger:
#  level: error
#  maskString: ****
#  appGroupName: app`

	uncommented := conv.UncommentYAML([]byte(input))
	result := string(uncommented)

	assert.Contains(t, result, `maskString: "****"`)
	assert.Contains(t, result, `level: error`)
	assert.Contains(t, result, `appGroupName: app`)
}

func TestLoadConfigFileAutoUncommentsTemplates(t *testing.T) {
	// Create a temporary fully-commented YAML file
	dir := t.TempDir()
	content := `# Config template
#server:
#  host: localhost
#  port: 8080
#  timeout: 30s
`
	filePath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	cmd := &Command{fromPath: dir}
	data, err := cmd.loadConfigFile(filePath, formatYAML, false)
	require.NoError(t, err)

	assert.NotEmpty(t, data)
	serverData, ok := data["server"].(map[string]any)
	require.True(t, ok, "expected 'server' key with nested map")
	assert.Equal(t, "localhost", serverData["host"])
	assert.Equal(t, 8080, serverData["port"])
	assert.Equal(t, "30s", serverData["timeout"])
}

func TestLoadConfigFilePreservesRealContent(t *testing.T) {
	// Create a temporary YAML with real (uncommented) content
	dir := t.TempDir()
	content := `server:
  host: production.example.com
  port: 9090
# commented:
#   key: value
`
	filePath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	cmd := &Command{fromPath: dir}
	data, err := cmd.loadConfigFile(filePath, formatYAML, false)
	require.NoError(t, err)

	// Only the uncommented content should be present
	serverData, ok := data["server"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "production.example.com", serverData["host"])
	assert.Equal(t, 9090, serverData["port"])
	// Commented-out "commented" key should NOT be present
	_, hasCommented := data["commented"]
	assert.False(t, hasCommented, "commented key should not be auto-uncommented in mixed files")
}

func TestLoadAndMergeParallelReturnsErrorOnEmpty(t *testing.T) {
	// Create a temp directory with enough empty YAML files to trigger parallel processing
	dir := t.TempDir()
	for i := range parallelProcessingThreshold + 1 {
		name := filepath.Join(dir, "config"+string(rune('a'+i))+".yaml")
		require.NoError(t, os.WriteFile(name, []byte("# just a comment\n"), 0o644))
	}

	cmd := &Command{fromPath: dir}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	_, err = cmd.loadAndMergeParallel(entries, formatYAML, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no valid config files found")
}

func TestConvertDataWithCommentsTemplateYAML(t *testing.T) {
	conv := NewConfigToEnvConverter()
	data := map[string]any{
		"logger": map[string]any{
			"level":      "error",
			"maskString": "****",
		},
	}

	outPath := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, conv.ConvertData(data, outPath))

	content, err := os.ReadFile(outPath)
	require.NoError(t, err)

	output := string(content)
	assert.Contains(t, output, "LOGGER__LEVEL=error")
	assert.Contains(t, output, "LOGGER__MASK_STRING=****")
}

func TestFullEndToEndDirectoryConversion(t *testing.T) {
	dir := t.TempDir()

	// Create multiple fully-commented template files
	file1 := `# Server configuration template
#server:
#  host: 0.0.0.0
#  port: 8080
`
	file2 := `# Database configuration template
#database:
#  host: localhost
#  port: 5432
#  name: mydb
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "server.yaml"), []byte(file1), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "database.yaml"), []byte(file2), 0o644))

	outPath := filepath.Join(t.TempDir(), ".env")
	cmd := &Command{
		fromPath: dir,
		toPath:   outPath,
	}

	// Simulate the directory conversion flow
	mergedData, err := cmd.loadAndMergeDirectory(formatYAML, false)
	require.NoError(t, err)

	conv := NewConfigToEnvConverter()
	require.NoError(t, conv.ConvertData(mergedData, outPath))

	content, err := os.ReadFile(outPath)
	require.NoError(t, err)

	output := string(content)
	assert.True(t, len(output) > 0, "output should not be empty")

	// Top-level keys are preserved as-is (isMapOfStructs detects multiple map entries)
	assert.Contains(t, output, "database__HOST=localhost")
	assert.Contains(t, output, "database__PORT=5432")
	assert.Contains(t, output, "database__NAME=mydb")
	assert.Contains(t, output, "server__HOST=0.0.0.0")
	assert.Contains(t, output, "server__PORT=8080")
}

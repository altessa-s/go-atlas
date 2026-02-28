// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3

import (
	"bufio"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

const (
	maxIncludeDepth         = 10
	includeDirective        = "!include"
	includeRegexMatchGroups = 3 // full match + indent + path
)

var includeRegex = regexp.MustCompile(`^(\s*)` + regexp.QuoteMeta(includeDirective) + `\s+(.+)$`)

// Preprocess performs YAML-specific pre-processing, such as handling !include directives.
func (b *Backend) Preprocess(content, currentDir, rootDir string) (string, error) {
	return processIncludes(content, currentDir, 0, map[string]bool{}, rootDir)
}

// processIncludes recursively processes !include directives in the given content.
// It handles indentation and prevents infinite loops with a depth limit and visited map.
// rootDir is used to prevent path traversal by ensuring all includes are within it.
func processIncludes(content string, currentDir string, depth int, visited map[string]bool, rootDir string) (string, error) {
	if depth >= maxIncludeDepth {
		return "", fmt.Errorf("max include depth reached (%d)", maxIncludeDepth)
	}

	var result strings.Builder
	result.Grow(len(content))
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := scanner.Text()
		matches := includeRegex.FindStringSubmatch(line)

		if len(matches) == includeRegexMatchGroups {
			indent := matches[1]
			includePath := strings.TrimSpace(matches[2])

			// Resolve relative path
			if !filepath.IsAbs(includePath) {
				includePath = filepath.Join(currentDir, includePath)
			}

			// Clean path to handle .. and .
			includePath = filepath.Clean(includePath)

			// Security check: validate extension
			ext := strings.ToLower(filepath.Ext(includePath))
			if ext != ".yaml" && ext != ".yml" {
				return "", fmt.Errorf("security error: included file %s has invalid extension (only .yaml, .yml allowed)", includePath)
			}

			// Security check: path traversal protection
			rel, err := filepath.Rel(rootDir, includePath)
			if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
				return "", fmt.Errorf("security error: attempt to include file outside of config root: %s", includePath)
			}

			// Check for circular includes
			if visited[includePath] {
				return "", fmt.Errorf("circular include detected: %s", includePath)
			}

			// Create a new map for recursive calls to avoid side effects on the same level
			newVisited := make(map[string]bool, len(visited)+1)
			maps.Copy(newVisited, visited)
			newVisited[includePath] = true

			// Read included file
			includedContent, err := os.ReadFile(includePath)
			if err != nil {
				return "", coreerrs.WrapOperation(err, "read included file "+includePath)
			}

			// Recursively process includes in the included file
			processedIncludedContent, err := processIncludes(string(includedContent), filepath.Dir(includePath), depth+1, newVisited, rootDir)
			if err != nil {
				return "", err
			}

			// Apply indentation to included content
			indentedContent := applyIndentation(processedIncludedContent, indent)
			result.WriteString(indentedContent)
			if !strings.HasSuffix(indentedContent, "\n") {
				result.WriteString("\n")
			}
		} else {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	if err := scanner.Err(); err != nil {
		return "", coreerrs.Wrap(err, "error scanning content")
	}

	return result.String(), nil
}

// applyIndentation prepends the given indent to each line of the content,
// except for the first line and empty lines.
func applyIndentation(content, indent string) string {
	if indent == "" {
		return content
	}

	lines := strings.Split(content, "\n")
	result := make([]string, len(lines))

	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			result[i] = ""
		} else {
			result[i] = indent + line
		}
	}

	return strings.Join(result, "\n")
}

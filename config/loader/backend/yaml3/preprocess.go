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

			// Security check: validate extension on the symlink path.
			// A second extension check on the RESOLVED target runs below
			// — both are needed: the symlink-name check catches obvious
			// "!include foo.txt" mistakes at parse time, the resolved
			// check catches symlinks that lie about their target.
			ext := strings.ToLower(filepath.Ext(includePath))
			if ext != ".yaml" && ext != ".yml" {
				return "", fmt.Errorf("security error: included file %s has invalid extension (only .yaml, .yml allowed)", includePath)
			}

			// Resolve symlinks on the include path BEFORE the traversal
			// check. Without this, a symlink `legit.yaml -> /etc/shadow`
			// placed inside rootDir would pass both the extension check
			// (symlink name ends in .yaml) and the filepath.Rel-based
			// traversal check (the cleaned path resides inside
			// rootDir), and os.ReadFile would then follow the symlink
			// and inline the secret file. EvalSymlinks collapses every
			// link in the chain to the real target so security checks
			// operate on what will actually be read.
			//
			// When EvalSymlinks fails (missing file, broken symlink,
			// virtual path) we fall back to the cleaned includePath:
			// the downstream Rel-check + ReadFile will produce the same
			// errors they always did. This keeps the user-facing error
			// contract stable for the non-symlink cases — the symlink
			// attack itself REQUIRES the target to be readable, so a
			// failed EvalSymlinks could not have led to a successful
			// read either.
			//
			// Critical: resolve BOTH paths together or NEITHER. On
			// systems where /tmp is a symlink (e.g. /private/tmp on
			// macOS), mixing a resolved rootDir with an unresolved
			// includePath gives them different prefixes and the Rel
			// check rejects perfectly legal cases.
			realIncludePath := includePath
			realRootDir := rootDir
			if resolvedInclude, evalErr := filepath.EvalSymlinks(includePath); evalErr == nil {
				if resolvedRoot, evalRootErr := filepath.EvalSymlinks(rootDir); evalRootErr == nil {
					realIncludePath = resolvedInclude
					realRootDir = resolvedRoot
				}
			}

			// Security check: path traversal protection on RESOLVED paths.
			rel, err := filepath.Rel(realRootDir, realIncludePath)
			if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
				return "", fmt.Errorf("security error: attempt to include file outside of config root: %s", includePath)
			}

			// Security check: re-validate the extension on the
			// resolved target. The symlink name may end in .yaml while
			// the actual file does not — without this re-check, a
			// symlink inside rootDir pointing to (say) /etc/shadow
			// could still be inlined.
			realExt := strings.ToLower(filepath.Ext(realIncludePath))
			if realExt != ".yaml" && realExt != ".yml" {
				return "", fmt.Errorf("security error: included file %s resolves to non-YAML target %s", includePath, realIncludePath)
			}

			// Check for circular includes using the resolved path so
			// two symlinks pointing to the same file are detected.
			if visited[realIncludePath] {
				return "", fmt.Errorf("circular include detected: %s", includePath)
			}

			// Create a new map for recursive calls to avoid side effects on the same level
			newVisited := make(map[string]bool, len(visited)+1)
			maps.Copy(newVisited, visited)
			newVisited[realIncludePath] = true

			// Read the RESOLVED file. Using realIncludePath instead of
			// includePath also locks in the path between the security
			// checks and the actual read (TOCTOU defense — a symlink
			// swap between checks could otherwise smuggle a different
			// target through).
			includedContent, err := os.ReadFile(realIncludePath)
			if err != nil {
				return "", coreerrs.WrapOperation(err, "read included file "+includePath)
			}

			// Recursively process includes in the included file
			processedIncludedContent, err := processIncludes(string(includedContent), filepath.Dir(realIncludePath), depth+1, newVisited, rootDir)
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
